package opaauthorizerequest

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	opasdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/tracing/tracingtest"

	"github.com/zalando/skipper/filters/openpolicyagent"
)

// rawRequest opens a fresh TCP connection to addr, writes wire verbatim, and
// returns the parsed HTTP response.
func rawRequest(t *testing.T, addr, wire string) *http.Response {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := io.WriteString(conn, wire); err != nil {
		t.Fatalf("write wire: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

func TestSkipperOPABypassPoC(t *testing.T) {
	for _, tt := range []struct {
		name             string
		wireFmt          string
		wantStatus       int
		wantUpstrm       bool
		wantUpstreamBody string
	}{
		{
			name: "1: Content-Length benign body -> 200 ALLOW",
			wireFmt: "POST /priv HTTP/1.1\r\n" +
				"Host: %s\r\n" +
				"Content-Type: application/json\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 15\r\n" +
				"\r\n" +
				`{"admin":false}`,
			wantStatus:       200,
			wantUpstrm:       true,
			wantUpstreamBody: `{"admin":false}`,
		},
		{
			name: "2: Content-Length admin body -> 403 DENY (negative control)",
			wireFmt: "POST /priv HTTP/1.1\r\n" +
				"Host: %s\r\n" +
				"Content-Type: application/json\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 14\r\n" +
				"\r\n" +
				`{"admin":true}`,
			wantStatus:       403,
			wantUpstrm:       false,
			wantUpstreamBody: "",
		},
		{
			name: "3: chunked admin body -> EXPECTED 403, BUG 200 (fixed nobypass)",
			wireFmt: "POST /priv HTTP/1.1\r\n" +
				"Host: %s\r\n" +
				"Content-Type: application/json\r\n" +
				"Connection: close\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"\r\n" +
				"e\r\n" +
				`{"admin":true}` + "\r\n" +
				"0\r\n" +
				"\r\n",
			wantStatus:       403,
			wantUpstrm:       false,
			wantUpstreamBody: "",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			var upstreamBody string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				upstreamBody = string(b)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("UPSTREAM-REACHED"))
			}))
			defer upstream.Close()

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
			defer opaControlPlane.Stop()

			config := fmt.Appendf(nil, `{
		"services": {
			"test": {
				"url": %q
			}
		},
		"bundles": {
			"test": {
				"resource": "/bundles/{{ .bundlename }}"
			}
		},
		"labels": {
			"environment": "test"
		},
		"plugins": {
			"envoy_ext_authz_grpc": {
				"path": "envoy/authz/allow",
				"dry-run": false
			}
		}
	}`, opaControlPlane.URL())

			opts := []func(*openpolicyagent.OpenPolicyAgentInstanceConfig) error{
				openpolicyagent.WithConfigTemplate(config),
			}

			opaFactory, err := openpolicyagent.NewOpenPolicyAgentRegistry(
				openpolicyagent.WithTracer(tracingtest.NewTracer()),
				openpolicyagent.WithOpenPolicyAgentInstanceConfig(opts...),
			)
			if err != nil {
				t.Fatalf("registry: %v", err)
			}

			fr := make(filters.Registry)
			fr.Register(NewOpaAuthorizeRequestWithBodySpec(opaFactory))
			fr.Register(builtin.NewSetPath())

			r := eskip.MustParse(fmt.Sprintf(
				`* -> opaAuthorizeRequestWithBody("test") -> "%s"`, upstream.URL))

			proxy := proxytest.New(fr, r...)
			defer proxy.Close()

			host := strings.TrimPrefix(proxy.URL, "http://")

			resp := rawRequest(t, host, fmt.Sprintf(tt.wireFmt, host))

			reached := upstreamBody != ""

			t.Logf("[%s] status=%d upstreamReached=%v upstreamBody=%q",
				tt.name, resp.StatusCode, reached, upstreamBody)

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("[%s] status = %d, want %d", tt.name, resp.StatusCode, tt.wantStatus)
			}
			if reached != tt.wantUpstrm {
				t.Errorf("[%s] upstreamReached = %v, want %v", tt.name, reached, tt.wantUpstrm)
			}

			if upstreamBody != tt.wantUpstreamBody {
				t.Errorf("Failed to get expected upstream body want %q, got: %q", tt.wantUpstreamBody, upstreamBody)
			}
		})
	}
}
