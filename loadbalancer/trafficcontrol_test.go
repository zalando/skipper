package loadbalancer_test

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

const docWithTrafficControl = `
// for reproducibility:
// - we represent Traffic(1) here with a Host that always matches
// - we don't define member routes, so that the randomness in the routing table doesn't
//   prevent us from reproducing the problem with 100% chance. This way we expect 404
//   instead of the 500 indicating the infinite loopback.

group1:
	Host(".*") &&
	LBGroup("group1")
	-> lbDecide("group1", 2)
	-> <loopback>;

group2:
	LBGroup("group2")
	-> lbDecide("group2", 2)
	-> <loopback>;
`

func TestLoadBalancerWithTrafficControl(t *testing.T) {
	r, err := eskip.Parse(docWithTrafficControl)
	if err != nil {
		t.Fatal(err)
	}

	p := proxytest.New(builtin.MakeRegistry(), r...)
	defer p.Close()

	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusNotFound {
		t.Fatal("invalid status code received", rsp.StatusCode)
	}
}
