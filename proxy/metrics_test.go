package proxy_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/dnstest"
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

func TestMeasureProxyWatch(t *testing.T) {
	m := &metricstest.MockMetrics{}
	defer m.Close()

	tp := proxytest.Config{
		Routes: eskip.MustParse(`test: * -> latency("10ms") -> backendLatency("20ms") -> status(200) -> <shunt>`),
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
		},
		ProxyParams: proxy.Params{
			Metrics: m,
		},
	}.Create()
	defer tp.Close()

	client := tp.Client()
	rsp, body, err := client.GetBody(tp.URL + "/hello")
	require.NoError(t, err)
	require.Equal(t, 200, rsp.StatusCode)
	require.Equal(t, []byte(""), body)

	m.WithMeasures(func(measures map[string][]time.Duration) {
		assert.Equal(t, len(measures), 3)
		assert.Len(t, measures["proxy.total.duration"], 1)
		assert.Len(t, measures["proxy.request.duration"], 1)
		assert.Len(t, measures["proxy.response.duration"], 1)
		assert.InDelta(t, measures["proxy.total.duration"][0].Seconds(), 0.001, 0.001)
		assert.InDelta(t, measures["proxy.request.duration"][0].Seconds(), 0.001, 0.001)
		assert.InDelta(t, measures["proxy.response.duration"][0].Seconds(), 0.001, 0.001)
	})
}

func TestMeasureResponseSize(t *testing.T) {
	dnstest.LoopbackNames(t, "foo.skipper.test", "bar.skipper.test")

	m := &metricstest.MockMetrics{}
	defer m.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		size, err := strconv.Atoi(r.URL.Query().Get("size"))
		if err != nil {
			http.Error(w, "Invalid size parameter", http.StatusBadRequest)
			return
		}
		w.Write([]byte(strings.Repeat("x", size)))
	}))
	defer backend.Close()

	p := proxytest.Config{
		Routes: eskip.MustParse(fmt.Sprintf(`
			foo: Host("^foo[.]skipper[.]test") -> "%s";
			bar: Host("^bar[.]skipper[.]test") -> "%s";
		`, backend.URL, backend.URL)),
		ProxyParams: proxy.Params{Metrics: m},
	}.Create()
	defer p.Close()

	client := p.Client()
	get := func(url string) {
		t.Helper()
		rsp, _, err := client.GetBody(url)
		require.NoError(t, err)
		require.Equal(t, 200, rsp.StatusCode)
	}

	fooHost := net.JoinHostPort("foo.skipper.test", p.Port)
	barHost := net.JoinHostPort("bar.skipper.test", p.Port)

	get("http://" + fooHost + "/?size=1000")
	get("http://" + fooHost + "/?size=1234")
	get("http://" + barHost + "/?size=555")
	get("http://" + barHost + "/?size=77777")

	m.WithValues(func(values map[string][]float64) {
		assert.Equal(t, []float64{1000, 1234}, values[fmt.Sprintf("response.%s.size_bytes", fooHost)])
		assert.Equal(t, []float64{555, 77777}, values[fmt.Sprintf("response.%s.size_bytes", barHost)])
	})
}

func TestMeasureBackendRequestHeader(t *testing.T) {
	dnstest.LoopbackNames(t, "foo.skipper.test", "bar.skipper.test")

	m := &metricstest.MockMetrics{}
	defer m.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	var (
		fooHeader = strings.Repeat("A", 100)
		barHeader = strings.Repeat("B", 200)
	)

	p := proxytest.Config{
		Routes: eskip.MustParse(fmt.Sprintf(`
			foo: Host("^foo[.]skipper[.]test") -> setRequestHeader("Foo", "%s") -> "%s";
			bar: Host("^bar[.]skipper[.]test") -> setRequestHeader("Bar", "%s") -> "%s";
		`, fooHeader, backend.URL, barHeader, backend.URL)),
		RoutingOptions: routing.Options{FilterRegistry: builtin.MakeRegistry()},
		ProxyParams:    proxy.Params{Metrics: m},
	}.Create()
	defer p.Close()

	client := p.Client()
	get := func(url string) {
		t.Helper()
		rsp, _, err := client.GetBody(url)
		require.NoError(t, err)
		require.Equal(t, 200, rsp.StatusCode)
	}

	fooHost := net.JoinHostPort("foo.skipper.test", p.Port)
	barHost := net.JoinHostPort("bar.skipper.test", p.Port)

	get("http://" + fooHost)
	get("http://" + barHost)

	m.WithValues(func(values map[string][]float64) {
		fooSize := values[fmt.Sprintf("backend.%s.request_header_bytes", fooHost)][0]
		barSize := values[fmt.Sprintf("backend.%s.request_header_bytes", barHost)][0]

		assert.Equal(t, barSize-fooSize, float64(len(barHeader)-len(fooHeader)))
	})
}
