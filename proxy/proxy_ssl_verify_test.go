package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/filters/builtin"
)

// TestProxySSLVerifyOff tests all four combinations of the global AllowInsecureBackend
// flag and the proxySSLVerifyOff filter being present or absent in the route.
func TestProxySSLVerifyOff(t *testing.T) {
	// httptest.NewTLSServer uses a self-signed cert not trusted by the default pool.
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	fr := builtin.MakeRegistry()

	for _, tc := range []struct {
		name                 string
		allowInsecureBackend bool
		filterInRoute        string
		wantStatus           int
	}{
		{
			name:                 "global_off_no_filter",
			allowInsecureBackend: false,
			filterInRoute:        "",
			wantStatus:           http.StatusInternalServerError,
		},
		{
			name:                 "global_off_with_filter",
			allowInsecureBackend: false,
			filterInRoute:        "proxySSLVerifyOff() -> ",
			wantStatus:           http.StatusInternalServerError,
		},
		{
			name:                 "global_on_no_filter",
			allowInsecureBackend: true,
			filterInRoute:        "",
			wantStatus:           http.StatusInternalServerError,
		},
		{
			name:                 "global_on_with_filter",
			allowInsecureBackend: true,
			filterInRoute:        "proxySSLVerifyOff() -> ",
			wantStatus:           http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fmt.Sprintf(`* -> %s"%s"`, tc.filterInRoute, backend.URL)
			params := Params{
				AllowInsecureBackend: tc.allowInsecureBackend,
				CloseIdleConnsPeriod: -1,
			}
			tp, err := newTestProxyWithFiltersAndParams(fr, doc, params, nil)
			if err != nil {
				t.Fatalf("failed to create test proxy: %v", err)
			}
			defer tp.close()

			ps := httptest.NewServer(tp.proxy)
			defer ps.Close()

			rsp, err := ps.Client().Get(ps.URL + "/")
			if err != nil {
				t.Fatalf("proxy request failed: %v", err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, rsp.StatusCode)
			}
		})
	}
}

// TestProxySSLVerifyOff_GlobalFlagGatesInsecureTransport verifies that insecureRoundTripper
// is nil when AllowInsecureBackend=false and non-nil when AllowInsecureBackend=true.
func TestProxySSLVerifyOff_GlobalFlagGatesInsecureTransport(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		p := WithParams(Params{AllowInsecureBackend: false, CloseIdleConnsPeriod: -1})
		defer p.Close()
		if p.insecureRoundTripper != nil {
			t.Error("expected insecureRoundTripper to be nil when AllowInsecureBackend=false")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		p := WithParams(Params{AllowInsecureBackend: true, CloseIdleConnsPeriod: -1})
		defer p.Close()
		if p.insecureRoundTripper == nil {
			t.Error("expected insecureRoundTripper to be non-nil when AllowInsecureBackend=true")
		}
	})
}
