package loadbalancer_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/loadbalancer"
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

func TestConcurrency(t *testing.T) {
	const (
		backendCount     = 7
		concurrency      = 32
		repeatedRequests = 300

		// 5% tolerated
		distributionTolerance = concurrency * repeatedRequests * 5 / 100
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

	for member, counter := range memberCounters {
		for compareMember, compare := range memberCounters {
			d := counter.value() - compare.value()
			if d < 0 {
				d = 0 - d
			}

			if d > distributionTolerance {
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
