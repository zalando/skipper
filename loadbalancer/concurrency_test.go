package loadbalancer_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
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

func TestConcurrencySingleRoute(t *testing.T) {
	const (
		backendCount     = 7
		concurrency      = 32
		repeatedRequests = 300

		// 5% tolerated
		distributionTolerance = concurrency * repeatedRequests / backendCount * 5 / 100
	)

	startBackend := func(content []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write(content)
		}))
	}

	var (
		contents []string
		backends []string
	)

	for i := 0; i < backendCount; i++ {
		content := fmt.Sprintf("lb group member %d", i)
		contents = append(contents, content)

		b := startBackend([]byte(content))
		defer b.Close()
		backends = append(backends, b.URL)
	}

	baseRoute := &eskip.Route{
		Id: "foo",
	}

	routes := loadbalancer.BalanceRoute(baseRoute, backends)
	p := proxytest.New(builtin.MakeRegistry(), routes...)
	defer p.Close()

	memberCounters := make(map[string]counter)
	for i := range contents {
		memberCounters[contents[i]] = newCounter()
	}

	var wg sync.WaitGroup
	runClient := func() {
		for i := 0; i < 300 && !t.Failed(); i++ {
			req, err := http.NewRequest("GET", p.URL, nil)
			if err != nil {
				t.Fatal(err)
				break
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				break
			}

			defer rsp.Body.Close()
			if rsp.StatusCode != http.StatusOK {
				t.Error("invalid status code", rsp.StatusCode)
				break
			}

			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Error(err)
				break
			}

			c, ok := memberCounters[string(b)]
			if !ok {
				t.Error("invalid response content", string(b))
				break
			}

			c.inc()
		}

		wg.Done()
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go runClient()
	}

	wg.Wait()
	if t.Failed() {
		return
	}

	checkDistribution(t, memberCounters, distributionTolerance)
}

func TestConstantlyUpdatingRoutes(t *testing.T) {
	const (
		backendCount       = 7
		concurrency        = 32
		repeatedRequests   = 300
		routeUpdateTimeout = 5 * time.Millisecond
		// 5% tolerated
		distributionTolerance = concurrency * repeatedRequests / backendCount * 5 / 100
	)

	startBackend := func(content []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write(content)
		}))
	}

	var (
		contents []string
		backends []string
	)

	for i := 0; i < backendCount; i++ {
		content := fmt.Sprintf("lb group member %d", i)
		contents = append(contents, content)

		b := startBackend([]byte(content))
		defer b.Close()
		backends = append(backends, b.URL)
	}

	baseRoute := &eskip.Route{
		Id:      "foo",
		Backend: "https://foo",
	}

	routes := loadbalancer.BalanceRoute(baseRoute, backends)
	dataClient := createDataClientWithUpdates(routes, routeUpdateTimeout)

	p := proxytest.WithRoutingOptions(builtin.MakeRegistry(), routing.Options{
		DataClients: []routing.DataClient{dataClient},
		PollTimeout: routeUpdateTimeout,
	}, routes...)
	defer p.Close()

	memberCounters := make(map[string]counter)
	for i := range contents {
		memberCounters[contents[i]] = newCounter()
	}

	var wg sync.WaitGroup
	runClient := func() {
		ticker := time.NewTicker(routeUpdateTimeout)
		for i := 0; i < repeatedRequests && !t.Failed(); i++ {
			select {
			case <-ticker.C:
				req, err := http.NewRequest("GET", p.URL, nil)
				if err != nil {
					t.Fatal(err)
					break
				}

				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					break
				}

				defer rsp.Body.Close()
				if rsp.StatusCode != http.StatusOK {
					t.Error("invalid status code", rsp.StatusCode)
					break
				}

				b, err := ioutil.ReadAll(rsp.Body)
				if err != nil {
					t.Error(err)
					break
				}

				c, ok := memberCounters[string(b)]
				if !ok {
					t.Error("invalid response content", string(b))
					break
				}
				c.inc()
			}
		}

		wg.Done()
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go runClient()
	}

	wg.Wait()
	if t.Failed() {
		return
	}

	checkDistribution(t, memberCounters, distributionTolerance)
}

func TestConcurrencyMultipleRoutes(t *testing.T) {
	const (
		backendCount     = 7
		concurrency      = 32
		repeatedRequests = 300

		// 5% tolerated
		distributionTolerance = concurrency * repeatedRequests / backendCount * 5 / 100
	)

	startBackend := func(content []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(time.Duration(rand.Intn(100)) * time.Microsecond)
			w.Write(content)
		}))
	}

	var (
		contents   = make(map[string][]string)
		backends   = make(map[string][]string)
		baseRoutes = make(map[string]*eskip.Route)
		routes     []*eskip.Route
	)

	apps := []string{"app1", "app2"}
	for _, app := range apps {
		for i := 0; i < backendCount; i++ {
			content := fmt.Sprintf("%s lb group member %d", app, i)
			contents[app] = append(contents[app], content)

			b := startBackend([]byte(content))
			defer b.Close()
			backends[app] = append(backends[app], b.URL)
		}
	}

	for _, app := range apps {
		baseRoutes[app] = &eskip.Route{
			Id:   app,
			Path: fmt.Sprintf("/%s", app),
		}
		routes = append(routes, loadbalancer.BalanceRoute(baseRoutes[app], backends[app])...)
	}

	p := proxytest.New(builtin.MakeRegistry(), routes...)
	defer p.Close()

	var memberCounters map[string]map[string]counter
	memberCounters = make(map[string]map[string]counter)

	for _, app := range apps {
		memberCounters[app] = make(map[string]counter)
		for i := range contents[app] {
			memberCounters[app][contents[app][i]] = newCounter()
		}
	}

	var wg sync.WaitGroup
	runClient := func() {
		for i := 0; i < 300 && !t.Failed(); i++ {
			curApp := apps[i%2]
			req, err := http.NewRequest("GET", p.URL+"/"+curApp, nil)
			if err != nil {
				t.Fatal(err)
				break
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				break
			}

			defer rsp.Body.Close()
			if rsp.StatusCode != http.StatusOK {
				t.Error("invalid status code", rsp.StatusCode)
				break
			}

			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Error(err)
				break
			}

			c, ok := memberCounters[curApp][string(b)]
			if !ok {
				t.Errorf("invalid response content for %s: %s", curApp, string(b))
				break
			}

			c.inc()
		}

		wg.Done()
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go runClient()
	}

	wg.Wait()
	if t.Failed() {
		return
	}

	for _, app := range apps {
		checkDistribution(t, memberCounters[app], distributionTolerance)
	}
}

func createDataClientWithUpdates(initial []*eskip.Route, updateTimeout time.Duration) *testdataclient.Client {
	dataClient := testdataclient.New(initial)

	ticker := time.NewTicker(updateTimeout)
	go func() {
		for {
			select {
			case <-ticker.C:
				dataClient.Update(append(initial, &eskip.Route{
					Id:      "meaningless_route",
					Backend: "https://some.site",
					Predicates: []*eskip.Predicate{{
						Name: "Host",
						Args: []interface{}{
							"no.sense",
						},
					}},
				}), []string{})
			}
		}
	}()

	return dataClient
}

func checkDistribution(t *testing.T, counters map[string]counter, tolerance int) {
	for member, counter := range counters {
		for compareMember, compare := range counters {
			d := counter.value() - compare.value()
			if d < 0 {
				d = 0 - d
			}

			if d > tolerance {
				t.Error(
					"failed to equally balance load, counters:",
					member,
					counter.value(),
					compareMember,
					compare.value(),
				)
			}
		}
	}
}
