package opaauthorizerequest

// Test for GHSA-j3r3-rjcc-vh4q: OPA body inspection bypass via Content-Encoding: gzip.
//
// When a request body is gzip-compressed (Content-Encoding: gzip), Go's net/http
// does not auto-decompress it. ExtractHttpBodyOptionally passes the raw compressed
// bytes to OPA, so input.parsed_body is null and deny-on-body-field Rego policies
// fail open. Additionally, the semaphore is acquired for only the compressed size,
// allowing far more concurrent requests than intended.
//

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

// buildGzipBypassProxy creates a proxy backed by an OPA instance that denies
// requests whose parsed body contains admin == true.  extraOpts are appended
// to the registry options (use them to set maxBodyBytes etc.).
func buildGzipBypassProxy(
	t *testing.T,
	upstreamHandler http.HandlerFunc,
	extraOpts ...func(*openpolicyagent.OpenPolicyAgentRegistry) error,
) *proxytest.TestProxy {
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
	registryOpts = append(registryOpts, extraOpts...)

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

// TestGzipBodyPolicyBypass is failure-mode-A (primary).
// A gzip-encoded body containing {"admin":true} must be denied (403).
// Without the fix, ExtractHttpBodyOptionally hands raw compressed bytes to OPA;
// input.parsed_body is null; the deny rule fails open and the proxy returns 200.
func TestGzipBodyPolicyBypass(t *testing.T) {
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
			name:               "plaintext admin body is denied (negative control)",
			body:               plaintext,
			contentEncoding:    "",
			wantStatus:         http.StatusForbidden,
			wantUpstreamCalled: false,
		},
		{
			name:               "gzip-encoded admin body must be denied",
			body:               compressed,
			contentEncoding:    "gzip",
			wantStatus:         http.StatusForbidden, // BUG before fix: returns 200
			wantUpstreamCalled: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalled bool
			proxy := buildGzipBypassProxy(t, func(w http.ResponseWriter, r *http.Request) {
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

// The decompressed body (280 bytes) exceeds maxBodyBytes (64 bytes), so the
// body is truncated and OPA receives x-envoy-auth-partial-body: true.
// A policy that denies partial bodies proves OPA saw decompressed-then-truncated
// bytes, not the raw compressed bytes that fit within maxBodyBytes unchanged.
func TestGzipBodySemaphoreAccuracy(t *testing.T) {
	// Produce a payload that is small when compressed but large when decompressed.
	// Repeating a short pattern compresses very well.
	plain := bytes.Repeat([]byte(`{"admin":true}`), 20) // 280 bytes plaintext
	compressed := gzipCompress(plain)
	if int64(len(compressed)) >= 64 {
		t.Skipf("gzip did not compress enough for this test (compressed=%d bytes)", len(compressed))
	}

	const maxBodyBytes = 64

	// Use a dedicated proxy whose policy denies requests with a partial (truncated) body.
	// x-envoy-auth-partial-body is set to "true" by the adapter when truncated=true,
	// which only happens when the decompressed size exceeds maxBodyBytes.
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

				allow := false if {
					input.attributes.request.http.headers["x-envoy-auth-partial-body"] == "true"
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
		openpolicyagent.WithMaxRequestBodyBytes(maxBodyBytes),
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

	// decompressed body (280 bytes) > maxBodyBytes (64 bytes) →
	// truncated → x-envoy-auth-partial-body: true → policy denies → 403.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("GHSA-j3r3-rjcc-vh4q semaphore: got %d, want 403; "+
			"expected gzip body (compressed %d bytes, decompressed %d bytes) to be "+
			"decompressed before semaphore acquisition so the %d-byte limit triggers "+
			"truncation and x-envoy-auth-partial-body: true",
			resp.StatusCode, len(compressed), len(plain), maxBodyBytes)
	}
}
