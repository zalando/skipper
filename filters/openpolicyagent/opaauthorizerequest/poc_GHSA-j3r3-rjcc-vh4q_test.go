package opaauthorizerequest

// Test for GHSA-j3r3-rjcc-vh4q: OPA body inspection bypass via Content-Encoding.
//
// When a request carries any Content-Encoding header, Go's net/http does not
// automatically decode the body. Passing encoded bytes to OPA means
// input.parsed_body is null and deny-on-body-field Rego policies fail open.
//
// The fix has two layers:
//   1. Any Content-Encoding header is rejected with 415 before body extraction.
//   2. Any non-JSON body bytes (e.g. compressed without Content-Encoding) are
//      rejected with 400, preventing silent null in input.parsed_body.

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"
)

// gzipCompress compresses src and returns the gzip bytes.
func gzipCompress(src []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(src)
	_ = w.Close()
	return buf.Bytes()
}

// buildBodyParsingProxy creates a proxy backed by an OPA instance that denies
// requests whose parsed body contains admin == true.
func buildBodyParsingProxy(t *testing.T, upstreamHandler http.HandlerFunc) *proxytest.TestProxy {
	t.Helper()

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz

				import rego.v1

				default allow := true

				allow := false if {
					input.parsed_body.admin == true
				}
			`,
		}),
	)
	t.Cleanup(opaControlPlane.Stop)

	config := fmt.Appendf(nil, `{
		"services": {"test": {"url": %q}},
		"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
		"labels": {"environment": "test"},
		"plugins": {"envoy_ext_authz_grpc": {"path": "envoy/authz/allow", "dry-run": false}}
	}`, opaControlPlane.URL())

	instanceOpts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}
	registryOpts := []func(*openpolicyagent.OpenPolicyAgentRegistry) error{
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(instanceOpts...),
	}

	opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(registryOpts...)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	fr := make(filters.Registry)
	fr.Register(NewOpaAuthorizeRequestWithBodySpec(opaFactory))
	fr.Register(builtin.NewSetPath())

	r := eskip.MustParse(fmt.Sprintf(
		`* -> opaAuthorizeRequestWithBody("test") -> "%s"`, upstream.URL))

	proxy := proxytest.New(fr, r...)
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy
}

// TestContentEncodingRejected verifies that any request carrying a
// Content-Encoding header is rejected with 415 Unsupported Media Type when
// body parsing is enabled. This prevents policy bypass via encoded bodies
// (GHSA-j3r3-rjcc-vh4q).
func TestContentEncodingRejected(t *testing.T) {
	plaintext := []byte(`{"admin":true}`)
	compressed := gzipCompress(plaintext)

	for _, tc := range []struct {
		name               string
		body               []byte
		contentEncoding    string
		wantStatus         int
		wantUpstreamCalled bool
	}{
		{
			name:               "plaintext non-admin body is allowed",
			body:               []byte(`{"admin":false}`),
			contentEncoding:    "",
			wantStatus:         http.StatusOK,
			wantUpstreamCalled: true,
		},
		{
			name:               "plaintext admin body is denied by policy",
			body:               plaintext,
			contentEncoding:    "",
			wantStatus:         http.StatusForbidden,
			wantUpstreamCalled: false,
		},
		{
			name:               "gzip-encoded body is rejected with 415",
			body:               compressed,
			contentEncoding:    "gzip",
			wantStatus:         http.StatusUnsupportedMediaType,
			wantUpstreamCalled: false,
		},
		{
			name:               "br-encoded body is rejected with 415",
			body:               []byte(`fake-br-data`),
			contentEncoding:    "br",
			wantStatus:         http.StatusUnsupportedMediaType,
			wantUpstreamCalled: false,
		},
		{
			name:               "deflate-encoded body is rejected with 415",
			body:               []byte(`fake-deflate-data`),
			contentEncoding:    "deflate",
			wantStatus:         http.StatusUnsupportedMediaType,
			wantUpstreamCalled: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalled bool
			proxy := buildBodyParsingProxy(t, func(w http.ResponseWriter, r *http.Request) {
				upstreamCalled = true
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/priv", bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tc.contentEncoding)
			}

			resp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if upstreamCalled != tc.wantUpstreamCalled {
				t.Errorf("upstreamCalled = %v, want %v", upstreamCalled, tc.wantUpstreamCalled)
			}
		})
	}
}

// TestContentEncodingAllowedWithoutBodyParsing verifies that Content-Encoding
// is not rejected when body parsing is disabled (opaAuthorizeRequest without
// body). The filter only blocks encoded bodies when it would parse them.
func TestContentEncodingAllowedWithoutBodyParsing(t *testing.T) {
	compressed := gzipCompress([]byte(`{"admin":true}`))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz

				import rego.v1

				default allow := true
			`,
		}),
	)
	t.Cleanup(opaControlPlane.Stop)

	config := fmt.Appendf(nil, `{
		"services": {"test": {"url": %q}},
		"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
		"labels": {"environment": "test"},
		"plugins": {"envoy_ext_authz_grpc": {"path": "envoy/authz/allow", "dry-run": false}}
	}`, opaControlPlane.URL())

	instanceOpts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
		openpolicyagent.WithConfigTemplate(config),
	}
	registryOpts := []func(*openpolicyagent.OpenPolicyAgentRegistry) error{
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(instanceOpts...),
	}

	opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(registryOpts...)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	fr := make(filters.Registry)
	fr.Register(NewOpaAuthorizeRequestSpec(opaFactory)) // no body parsing
	fr.Register(builtin.NewSetPath())

	r := eskip.MustParse(fmt.Sprintf(
		`* -> opaAuthorizeRequest("test") -> "%s"`, upstream.URL))

	proxy := proxytest.New(fr, r...)
	t.Cleanup(func() { _ = proxy.Close() })

	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/priv", bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200; Content-Encoding should not be rejected when body parsing is disabled", resp.StatusCode)
	}
}

// TestNonJSONBodyRejected verifies that a body which is not valid JSON is
// rejected with 400 even when no Content-Encoding header is set. This closes
// the bypass where an attacker sends compressed (or otherwise non-JSON) bytes
// with Content-Type: application/json but omits Content-Encoding.
func TestNonJSONBodyRejected(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       []byte
		wantStatus int
	}{
		{
			name:       "valid JSON is accepted",
			body:       []byte(`{"admin":false}`),
			wantStatus: http.StatusOK,
		},
		{
			name:       "gzip bytes without Content-Encoding are rejected with 400",
			body:       gzipCompress([]byte(`{"admin":true}`)),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "arbitrary non-JSON bytes are rejected with 400",
			body:       []byte{0x00, 0x01, 0x02, 0x03},
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalled bool
			proxy := buildBodyParsingProxy(t, func(w http.ResponseWriter, r *http.Request) {
				upstreamCalled = true
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/priv", bytes.NewReader(tc.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK && upstreamCalled {
				t.Errorf("upstream must not be called on rejection")
			}
		})
	}
}
