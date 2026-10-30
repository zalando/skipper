package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	skpnet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func TestClientRequests(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		internal bool
		forged   bool
		want     int64
	}{
		{name: "noncache", path: "/normal", want: 1},
		{name: "no route", path: "/missing", want: 1},
		{name: "backend failure", path: "/failure", want: 1},
		{name: "uncacheable", path: "/private", want: 1},
		{name: "configured healthcheck", path: "/probe"},
		{name: "name is not identity", path: "/healthz", want: 1},
		{name: "internal", path: "/normal", internal: true},
		{name: "forged marker", path: "/normal", forged: true, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &metricstest.MockMetrics{}
			p := proxytest.Config{
				Routes: eskip.MustParse(`
					normal: Path("/normal") -> status(200) -> <shunt>;
					private: Path("/private") -> setResponseHeader("Cache-Control", "no-store") -> <shunt>;
					failure: Path("/failure") -> "http://127.0.0.1:0";
					probe: Path("/probe") -> healthcheck() -> <shunt>;
					healthcheck: Path("/healthz") -> status(200) -> <shunt>;
				`),
				RoutingOptions: routing.Options{FilterRegistry: builtin.MakeRegistry()},
				ProxyParams:    proxy.Params{Metrics: m},
			}.Create()
			defer p.Close()
			req, err := http.NewRequest(http.MethodGet, p.URL+tc.path, nil)
			require.NoError(t, err)
			if tc.internal {
				skpnet.MarkInternalRequest(req)
			} else if tc.forged {
				req.Header.Set("Skipper-Internal-Request", "1")
				req.Header.Set("X-Cache-Revalidate", "1")
			}

			resp, err := p.Client().Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			m.WithCounters(func(counters map[string]int64) {
				assert.Equal(t, tc.want, counters["client.requests"])
				assert.Equal(t, int64(1), counters["incoming.HTTP/1.1"])
			})
		})
	}
}

func TestClientRequestsPrometheus(t *testing.T) {
	m := metrics.NewPrometheus(metrics.Options{})
	p := proxytest.Config{
		Routes:      eskip.MustParse(`normal: * -> <shunt>;`),
		ProxyParams: proxy.Params{Metrics: m},
	}.Create()
	defer p.Close()
	scrape := func() string {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		m.CreateHandler().ServeHTTP(rec, req)
		return rec.Body.String()
	}
	assert.Contains(t, scrape(), `skipper_custom_total{key="client.requests"} 0`)
	resp, err := p.Client().Get(p.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Contains(t, scrape(), `skipper_custom_total{key="client.requests"} 1`)
}
