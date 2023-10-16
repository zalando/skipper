package scheduler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestCleanupOnBackendErrors(t *testing.T) {
	doc := `
		aroute: *
		-> lifo(1, 1, "100ms")
		-> lifoGroup("foo", 1, 1, "100ms")
		-> lifo(2, 2, "200ms")
		-> lifoGroup("bar", 1, 1, "100ms")
		-> fifo(1, 1, "200ms")
		-> "http://test.invalid"
	`

	dc, err := testdataclient.NewDoc(doc)
	require.NoError(t, err)
	defer dc.Close()

	reg := scheduler.RegistryWith(scheduler.Options{})
	defer reg.Close()

	fr := make(filters.Registry)
	fr.Register(NewLIFO())
	fr.Register(NewLIFOGroup())
	fr.Register(NewFifo())

	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PostProcessors:  []routing.PostProcessor{reg},
	}

	rt := routing.New(ro)
	defer rt.Close()

	<-rt.FirstLoad()

	pr := proxy.WithParams(proxy.Params{
		Routing: rt,
	})
	defer pr.Close()

	ts := httptest.NewServer(pr)
	defer ts.Close()

	rsp, err := http.Get(ts.URL)
	require.NoError(t, err)
	rsp.Body.Close()

	var route *routing.Route
	{
		req, err := http.NewRequest("GET", ts.URL, nil)
		require.NoError(t, err)

		route, _ = rt.Get().Do(req)
		require.NotNil(t, route, "failed to lookup route")
	}

	for _, f := range route.Filters {
		if qf, ok := f.Filter.(interface{ GetQueue() *scheduler.Queue }); ok {
			status := qf.GetQueue().Status()
			assert.Equal(t, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0, Closed: false}, status)
		} else if qf, ok := f.Filter.(interface{ GetQueue() *scheduler.FifoQueue }); ok {
			status := qf.GetQueue().Status()
			assert.Equal(t, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0, Closed: false}, status)
		} else {
			t.Fatal("filter does not implement GetQueue()")
		}
	}
}
