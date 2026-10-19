package opaserveresponse

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
	"time"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/openpolicyagent"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func gzipBytes(src []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(src)
	_ = w.Close()
	return buf.Bytes()
}

// buildServeResponseProxy creates a proxy using opaServeResponseWithReqBody
// backed by a policy that echoes input.parsed_body as the response body.
func buildServeResponseProxy(t *testing.T, upstream http.Handler) *proxytest.TestProxy {
	t.Helper()

	upstreamSrv := httptest.NewServer(upstream)
	t.Cleanup(upstreamSrv.Close)

	opaControlPlane := opasdktest.MustNewServer(
		opasdktest.MockBundle("/bundles/test", map[string]string{
			"main.rego": `
				package envoy.authz

				import rego.v1

				default allow_object_req_body := {
					"allowed": false,
					"headers": {},
					"body": "denied",
					"http_status": 403
				}

				allow_object_req_body := response if {
					input.parsed_body.admin != true
					response := {
						"allowed": true,
						"headers": {},
						"body": "allowed",
						"http_status": 200
					}
				}
			`,
		}),
	)
	t.Cleanup(opaControlPlane.Stop)

	config := fmt.Appendf(nil, `{
		"services": {"test": {"url": %q}},
		"bundles": {"test": {"resource": "/bundles/{{ .bundlename }}"}},
		"labels": {"environment": "test"},
		"plugins": {"envoy_ext_authz_grpc": {"path": "envoy/authz/allow_object_req_body", "dry-run": false, "skip-request-body-parse": false}}
	}`, opaControlPlane.URL())

	opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)),
	)
	require.NoError(t, err)

	fr := make(filters.Registry)
	fr.Register(NewOpaServeResponseWithReqBodySpec(opaFactory))

	r := eskip.MustParse(fmt.Sprintf(`* -> opaServeResponseWithReqBody("test") -> "%s"`, upstreamSrv.URL))

	proxy := proxytest.New(fr, r...)
	t.Cleanup(func() { _ = proxy.Close() })

	require.Eventually(t, func() bool {
		inst, instErr := opaFactory.GetOrStartInstance("test")
		return instErr == nil && inst.Started() && inst.Healthy()
	}, 5*time.Second, 50*time.Millisecond, "OPA instance did not become ready")

	return proxy
}

// TestServeResponseContentEncodingRejected verifies that any request carrying a
// Content-Encoding header is rejected with 415 Unsupported Media Type when body
// parsing is enabled (opaServeResponseWithReqBody). This prevents policy bypass
// via encoded bodies (GHSA-j3r3-rjcc-vh4q).
func TestServeResponseContentEncodingRejected(t *testing.T) {
	plaintext := []byte(`{"admin":true}`)
	compressed := gzipBytes(plaintext)

	for _, tc := range []struct {
		name            string
		body            []byte
		contentEncoding string
		wantStatus      int
	}{
		{
			name:       "plaintext non-admin body is allowed",
			body:       []byte(`{"admin":false}`),
			wantStatus: http.StatusOK,
		},
		{
			name:       "plaintext admin body is denied by policy",
			body:       plaintext,
			wantStatus: http.StatusForbidden,
		},
		{
			name:            "gzip-encoded body is rejected with 415",
			body:            compressed,
			contentEncoding: "gzip",
			wantStatus:      http.StatusUnsupportedMediaType,
		},
		{
			name:            "br-encoded body is rejected with 415",
			body:            []byte(`fake-br-data`),
			contentEncoding: "br",
			wantStatus:      http.StatusUnsupportedMediaType,
		},
		{
			name:            "deflate-encoded body is rejected with 415",
			body:            []byte(`fake-deflate-data`),
			contentEncoding: "deflate",
			wantStatus:      http.StatusUnsupportedMediaType,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := buildServeResponseProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))

			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/priv", bytes.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if tc.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tc.contentEncoding)
			}

			resp, err := proxy.Client().Do(req)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

// TestServeResponseContentEncodingAllowedWithoutBodyParsing verifies that
// Content-Encoding is not rejected when body parsing is disabled
// (opaServeResponse without body). The filter only blocks encoded bodies when
// it would parse them.
func TestServeResponseContentEncodingAllowedWithoutBodyParsing(t *testing.T) {
	compressed := gzipBytes([]byte(`{"admin":true}`))

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

	opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(
		openpolicyagent.WithTracer(tracingtest.NewTracer()),
		openpolicyagent.WithOpenPolicyAgentInstanceConfig(openpolicyagent.WithConfigTemplate(config)),
	)
	require.NoError(t, err)

	fr := make(filters.Registry)
	fr.Register(NewOpaServeResponseSpec(opaFactory)) // no body parsing

	r := eskip.MustParse(fmt.Sprintf(`* -> opaServeResponse("test") -> <shunt>`))

	proxy := proxytest.New(fr, r...)
	t.Cleanup(func() { _ = proxy.Close() })

	require.Eventually(t, func() bool {
		inst, instErr := opaFactory.GetOrStartInstance("test")
		return instErr == nil && inst.Started() && inst.Healthy()
	}, 5*time.Second, 50*time.Millisecond, "OPA instance did not become ready")

	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/priv", bytes.NewReader(compressed))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := proxy.Client().Do(req)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		// opaServeResponse with `allow := true` returns 500 because the allow rule
		// evaluates to a boolean (not a structured response object), which the
		// opaServeResponse filter treats as a misconfigured policy. The important
		// thing is it is NOT 415 — Content-Encoding is not rejected when body
		// parsing is disabled.
		t.Errorf("status = %d, want 500; Content-Encoding should not be rejected when body parsing is disabled", resp.StatusCode)
	}
}

// TestServeResponseNonJSONBodyRejected verifies that a body which is not valid
// JSON is rejected with 400 even when no Content-Encoding header is set. This
// closes the bypass where an attacker sends compressed (or otherwise non-JSON)
// bytes with Content-Type: application/json but omits Content-Encoding.
func TestServeResponseNonJSONBodyRejected(t *testing.T) {
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
			body:       gzipBytes([]byte(`{"admin":true}`)),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "arbitrary non-JSON bytes are rejected with 400",
			body:       []byte{0x00, 0x01, 0x02, 0x03},
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := buildServeResponseProxy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))

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
		})
	}
}
