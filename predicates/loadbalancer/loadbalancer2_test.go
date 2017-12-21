package loadbalancer_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestConcurrency2(t *testing.T) {
	const distributionTolerance = 100

	c1 := newCounter()
	b1 := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		c1.inc()
	}))
	defer b1.Close()

	c2 := newCounter()
	b2 := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		c2.inc()
	}))
	defer b2.Close()

	const routesFmt = `
		routeADecide: LoadDecide("A", 2) -> <loopback>;
		routeA1:      LoadBalancer2("A", 0) -> status(200) -> "%s";
		routeA2:      LoadBalancer2("A", 1) -> status(200) -> "%s";
	`
	routes := fmt.Sprintf(routesFmt, b1.URL, b2.URL)

	r, err := eskip.Parse(routes)
	if err != nil {
		t.Fatal(err)
	}

	p := proxytest.New(builtin.MakeRegistry(), r...)
	defer p.Close()

	failureCount := newCounter()

	var wg sync.WaitGroup
	runClient := func(id string) {
		for i := 0; i < 300; i++ {
			func() {
				req, err := http.NewRequest("GET", p.URL, nil)
				if err != nil {
					t.Fatal(err)
				}

				req.Header.Set("X-Trace", fmt.Sprintf("%s-%d", id, i))

				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					failureCount.inc()
					t.Log(err)
					return
				}

				defer rsp.Body.Close()
				if rsp.StatusCode != http.StatusOK {
					failureCount.inc()
					// t.Log("invalid status code", rsp.StatusCode)
				}
			}()
		}

		wg.Done()
	}

	const concurrency = 32
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go runClient(fmt.Sprintf("client-%d", i))
	}

	wg.Wait()

	if failureCount.value() > 0 {
		t.Error("concurrent load balancing failed, failures:", failureCount.value())
	}

	t.Log(int(uint(c1.value()-c2.value())), c1.value(), c2.value())
	if int(uint(c1.value()-c2.value())) > distributionTolerance {
		t.Error("failed to equally balance load, counters:", c1.value(), c2.value())
	}
}
