package loadbalancer_test

import (
	"io/ioutil"
	"net/http"
	"sync"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestConcurrency2(t *testing.T) {
	const (
		concurrency      = 32
		repeatedRequests = 300

		// 5% tolerated
		distributionTolerance = concurrency * repeatedRequests * 5 / 100
	)

	const routes = `
		routeADecide: LBGroup("A") -> lbDecide("A", 7) -> <loopback>;
		routeA1:      LBMember("A", 0) -> inlineContent("lb group member 1") -> <shunt>;
		routeA2:      LBMember("A", 1) -> inlineContent("lb group member 2") -> <shunt>;
		routeA3:      LBMember("A", 2) -> inlineContent("lb group member 3") -> <shunt>;
		routeA4:      LBMember("A", 3) -> inlineContent("lb group member 4") -> <shunt>;
		routeA5:      LBMember("A", 4) -> inlineContent("lb group member 5") -> <shunt>;
		routeA6:      LBMember("A", 5) -> inlineContent("lb group member 6") -> <shunt>;
		routeA7:      LBMember("A", 6) -> inlineContent("lb group member 7") -> <shunt>;
	`

	r, err := eskip.Parse(routes)
	if err != nil {
		t.Fatal(err)
	}

	p := proxytest.New(builtin.MakeRegistry(), r...)
	defer p.Close()

	memberCounters := map[string]counter{
		"lb group member 1": newCounter(),
		"lb group member 2": newCounter(),
		"lb group member 3": newCounter(),
		"lb group member 4": newCounter(),
		"lb group member 5": newCounter(),
		"lb group member 6": newCounter(),
		"lb group member 7": newCounter(),
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
