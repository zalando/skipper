package routing_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func testBackend(token string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(token))
	}))
}

func bindClient(p *proxytest.TestProxy) func(string, ...string) bool {
	return func(path string, responses ...string) bool {
	remaining:
		for range responses {
			rsp, err := http.Get(p.URL + path)
			if err != nil {
				return false
			}

			defer rsp.Body.Close()
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return false
			}

			for i := range responses {
				if strings.TrimSpace(string(b)) != responses[i] {
					continue
				}

				responses = append(responses[:i], responses[i+1:]...)
				continue remaining
			}

			return false
		}

		return true
	}
}

func TestFallbackGroupLB(t *testing.T) {
	// two groups and a non-grouped route, altogether 1 + 2 + 2 = 5 backends
	// initially, all work
	// the non-grouped backend fails
	// -> the non-grouped route fails, the others work and don't fall back
	// recovers
	// a grouped backend fails
	// -> the non-grouped route works, the failing one falls back, the other grouped works
	// recovers
	// both backends from a group fail -> both routes fail
	// -> the group fails

	// start one ungrouped, and two grouped backends:
	nonGrouped := testBackend("non-grouped")
	groupA_BE1 := testBackend("group-A/BE-1")
	groupA_BE2 := testBackend("group-A/BE-2")
	groupB_BE1 := testBackend("group-B/BE-1")
	groupB_BE2 := testBackend("group-B/BE-2")

	type closer interface {
		Close()
	}
	for _, c := range []closer{nonGrouped, groupA_BE1, groupA_BE2, groupB_BE1, groupB_BE2} {
		defer c.Close()
	}

	// start a proxy:
	const routesFmt = `
		nonGrouped: Path("/") -> "%s";

		groupA: Path("/a") && LBGroup("group-a")
			-> lbDecide("group-a", 2)
			-> <loopback>;
		groupA_BE1: Path("/a") && LBMember("group-a", 0) -> "%s";
		groupA_BE2: Path("/a") && LBMember("group-a", 1) -> "%s";

		groupB: Path("/b") && LBGroup("group-b")
			-> lbDecide("group-b", 2)
			-> <loopback>;
		groupB_BE1: Path("/b") && LBMember("group-b", 0) -> "%s";
		groupB_BE2: Path("/b") && LBMember("group-b", 1) -> "%s";
	`

	routesDoc := fmt.Sprintf(
		routesFmt,
		nonGrouped.URL,
		groupA_BE1.URL,
		groupA_BE2.URL,
		groupB_BE1.URL,
		groupB_BE2.URL,
	)

	routes, err := eskip.Parse(routesDoc)
	if err != nil {
		t.Fatal(err)
	}

	p := proxytest.New(builtin.MakeRegistry(), routes...)
	defer p.Close()

	request := bindClient(p)

	t.Run("succeed and load balance initially", func(t *testing.T) {
		if !request("/", "non-grouped") ||
			!request("/a", "group-A/BE-1", "group-A/BE-2") ||
			!request("/b", "group-B/BE-1", "group-B/BE-2") {
			t.Error("failed to receive the right response")
		}
	})

	t.Run("one member in the group fails", func(t *testing.T) {
		groupA_BE1.Close()
		time.Sleep(300 * time.Millisecond)

		// group A responds two times from backend 2:
		if !request("/", "non-grouped") ||
			!request("/a", "group-A/BE-2", "group-A/BE-2") ||
			!request("/b", "group-B/BE-1", "group-B/BE-2") {
			t.Error("failed to receive the right response")
		}
	})

	t.Run("both members in the group fail", func(t *testing.T) {
		groupA_BE1.Close()
		groupA_BE2.Close()
		time.Sleep(300 * time.Millisecond)

		// group A responds two times with failure:
		if !request("/", "non-grouped") ||
			!request("/a", "Internal Server Error", "Internal Server Error") ||
			!request("/b", "group-B/BE-1", "group-B/BE-2") {
			t.Error("failed to receive the right response")
		}
	})
}
