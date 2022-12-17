package scheduler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	fifo "github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestFifoLoopback(t *testing.T) {
	dc, err := testdataclient.NewDoc(`
		main: Path("/") -> fifo(100, 100, "1s") -> setPath("/loop") -> <loopback>;
		loop: Path("/loop") -> fifo(100, 100, "1s") -> inlineContent("ok") -> <shunt>;
	`)
	require.NoError(t, err)
	defer dc.Close()

	filterRegistry := make(filters.Registry)
	filterRegistry.Register(fifo.NewFifo())
	filterRegistry.Register(builtin.NewSetPath())
	filterRegistry.Register(builtin.NewInlineContent())

	schedulerRegistry := scheduler.RegistryWith(scheduler.Options{})
	defer schedulerRegistry.Close()

	rt := routing.New(routing.Options{
		DataClients:     []routing.DataClient{dc},
		FilterRegistry:  filterRegistry,
		PreProcessors:   []routing.PreProcessor{schedulerRegistry.PreProcessor()},
		PostProcessors:  []routing.PostProcessor{schedulerRegistry},
		SignalFirstLoad: true,
	})
	defer rt.Close()

	<-rt.FirstLoad()

	pr := proxy.WithParams(proxy.Params{Routing: rt})
	defer pr.Close()

	tsp := httptest.NewServer(pr)
	defer tsp.Close()

	resp, err := http.Get(tsp.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "ok", string(content))
}
