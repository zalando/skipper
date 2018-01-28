package loadbalancer_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	// "github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/proxy/proxytest"
)

type counter chan int

func newCounter() counter {
	c := make(counter, 1)
	c <- 0
	return c
}

func (c counter) inc() {
	v := <-c
	c <- v + 1
}

func (c counter) value() int {
	v := <-c
	c <- v
	return v
}

func (c counter) String() string {
	return fmt.Sprint(c.value())
}

func TestConcurrent(t *testing.T) {
	// This test should be converted into the new predicate/filter style
	t.Skip()

	// two backends
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

	// proxy with two load balanced routes
	const routesFmt = `
		route1: LoadBalancer("A", 0, 2) -> status(200) -> "%s";
		route2: LoadBalancer("A", 1, 2) -> status(200) -> "%s";
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
					t.Log("invalid status code", rsp.StatusCode)
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

	/*
		for _, logs := range loadbalancer.CountNonMatched {
			if len(logs) > 1 {
				for i := range logs {
					t.Log(logs[i])
				}
			}
		}
	*/

	t.Error("just fail", c1, c2, failureCount)
}
