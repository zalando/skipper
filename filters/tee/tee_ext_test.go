package tee_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics/metricstest"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type myHandler struct {
	t      *testing.T
	name   string
	header http.Header
	body   string
	served chan struct{}
}

func newTestHandler(t *testing.T, name string) *myHandler {
	return &myHandler{
		t:      t,
		name:   name,
		served: make(chan struct{}),
	}
}

func (h *myHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		h.t.Error(err)
	}
	h.header = r.Header
	h.body = string(b)
	close(h.served)
}

func TestTeeRedirectLoop(t *testing.T) {
	originalHandler := newTestHandler(t, "original")
	originalServer := httptest.NewServer(originalHandler)
	originalUrl := originalServer.URL
	defer originalServer.Close()

	routeStrFmt := `route1: * -> tee("%s") -> "%s";`
	route := eskip.MustParse(fmt.Sprintf(routeStrFmt, "dummy", originalUrl))

	fr := builtin.MakeRegistry()
	dc, err := testdataclient.NewDoc("")
	if err != nil {
		t.Fatalf("Failed to create dc: %v", err)
	}
	defer dc.Close()

	mockMetrics := &metricstest.MockMetrics{}
	defer mockMetrics.Close()
	tl := loggingtest.New()
	defer tl.Close()
	r := routing.New(routing.Options{
		DataClients: []routing.DataClient{dc},
		Log:         tl})
	defer r.Close()

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: fr,
			DataClients:    []routing.DataClient{dc},
			Metrics:        mockMetrics,
			Predicates:     []routing.PredicateSpec{teePredicate.New()},
		},
		ProxyParams: proxy.Params{
			CloseIdleConnsPeriod: time.Second,
			Metrics:              mockMetrics,
		},
	}.Create()
	defer p.Close()

	// create the loop
	route = eskip.MustParse(fmt.Sprintf(routeStrFmt, p.URL, originalUrl))
	dc.Update(route, nil)
	time.Sleep(50 * time.Millisecond)

	testingStr := "TESTEST"
	req, err := http.NewRequest("GET", p.URL, strings.NewReader(testingStr))
	if err != nil {
		t.Error(err)
	}

	req.Host = "www.example.org"
	req.Close = true
	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Error(err)
	}
	mockMetrics.WithCounters(func(counters map[string]int64) {
		for k, v := range counters {
			t.Logf("%s: %d", k, v)
			if k == "incoming.HTTP/1.1" && v > 1 {
				t.Fatalf("Failed to mitigate duplicate request: %d", v)
			}
		}

	})

	rsp.Body.Close()
}
