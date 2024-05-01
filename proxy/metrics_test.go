package proxy_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestMetricsUncompressed(t *testing.T) {
	m := &metricstest.MockMetrics{}

	// will update routes after proxy address is known
	dc := testdataclient.New(nil)
	defer dc.Close()

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			DataClients:    []routing.DataClient{dc},
		},
		ProxyParams: proxy.Params{
			Metrics: m,
		},
	}.Create()
	defer p.Close()

	err := dc.UpdateDoc(fmt.Sprintf(`
		test: Path("/test") -> setPath("/backend") -> "%s";
		backend: Path("/backend") && Header("Accept-Encoding", "gzip") -> compress() -> inlineContent("backend response") -> <shunt>;
	`, p.URL), nil)
	require.NoError(t, err)

	// wait for route update
	time.Sleep(10 * time.Millisecond)

	client := p.Client()
	client.Transport.(*http.Transport).DisableCompression = true

	const N = 10

	for i := 0; i < N; i++ {
		rsp, body, err := client.GetBody(p.URL + "/test")
		require.NoError(t, err)

		require.Equal(t, 200, rsp.StatusCode)
		require.Equal(t, []byte("backend response"), body)
	}

	m.WithCounters(func(counters map[string]int64) {
		assert.Equal(t, counters["incoming.HTTP/1.1"], int64(2*N))
		assert.Equal(t, counters["outgoing.HTTP/1.1"], int64(N))
		assert.Equal(t, counters["experimental.uncompressed"], int64(N))
	})
}
