package scheduler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	filterScheduler "github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestFifoChanges(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	schedulerRegistry := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics.Default,
		EnableRouteFIFOMetrics: true,
	})
	defer schedulerRegistry.Close()

	spec := filterScheduler.NewFifo()
	fr := builtin.MakeRegistry()
	fr.Register(spec)
	args := []any{
		2,
		2,
		"2s",
	}

	_, err := spec.CreateFilter(args)
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	r := &eskip.Route{Id: "r_test", Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

	doc := fmt.Sprintf(`r_test: * -> fifo(%d, %d, "%s") -> "%s"`, append(args, backend.URL)...)
	cli, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Fatalf("Failed to create testdataclient: %v", err)
	}

	proxy := proxytest.WithRoutingOptions(fr, routing.Options{
		SignalFirstLoad: true,
		DataClients:     []routing.DataClient{cli},
		PreProcessors: []routing.PreProcessor{
			schedulerRegistry.PreProcessor(),
		},
		PostProcessors: []routing.PostProcessor{
			schedulerRegistry,
		},
	}, r)

	reqURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
	}

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		t.Error(err)
		return
	}

	errCH := make(chan error)
	f := func(t *testing.T, errCH chan<- error, r *http.Request, wantCode int) {
		t.Helper()

		client := net.NewClient(net.Options{
			ResponseHeaderTimeout: 2 * time.Second,
		})
		rsp, err := client.Do(r)
		if err != nil {
			errCH <- fmt.Errorf("Failed to do http call: %w", err)
			return
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != wantCode {
			errCH <- fmt.Errorf("fifo filter failed got=%d, want %d", rsp.StatusCode, wantCode)
			return
		}
	}
	time.Sleep(100 * time.Millisecond)
	N := 4
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			f(t, errCH, req, http.StatusOK)
			wg.Done()

		}()
	}
	time.Sleep(100 * time.Millisecond)
	go f(t, errCH, req, http.StatusServiceUnavailable)

	wg.Wait()
	close(errCH)
	for err := range errCH {
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	errCH = make(chan error)
	go func() {
		f(t, errCH, req, http.StatusOK)
		close(errCH)
	}()
	err = <-errCH
	if err != nil {
		t.Fatalf("Failed to get ok: %v", err)
	}
}
