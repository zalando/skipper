package proxy_test

// TODO: move this to the proxy package

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

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
	nonGrouped := testBackend("non-grouped", http.StatusOK)
	groupA_BE1 := testBackend("group-A/BE-1", http.StatusOK)
	groupA_BE2 := testBackend("group-A/BE-2", http.StatusOK)
	groupB_BE1 := testBackend("group-B/BE-1", http.StatusOK)
	groupB_BE2 := testBackend("group-B/BE-2", http.StatusOK)

	type closer interface {
		Close()
	}
	for _, c := range []closer{nonGrouped, groupA_BE1, groupA_BE2, groupB_BE1, groupB_BE2} {
		defer c.Close()
	}

	// start a proxy:
	const routesFmt = `
		nonGrouped: Path("/") -> "%s";

		groupA: Path("/a") -> <roundRobin, "%s", "%s">;

		groupB: Path("/b") -> <roundRobin, "%s", "%s">;
	`

	routesDoc := fmt.Sprintf(
		routesFmt,
		nonGrouped.URL,
		groupA_BE1.URL,
		groupA_BE2.URL,
		groupB_BE1.URL,
		groupB_BE2.URL,
	)

	routes := eskip.MustParse(routesDoc)

	p := proxytest.New(builtin.MakeRegistry(), routes...)
	defer p.Close()

	requestMSG := bindClient(p)
	request := func(path string, expectedResponses ...string) bool {
		_, ok := requestMSG(path, expectedResponses...)
		return ok
	}

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
			!request("/a", "Bad Gateway", "Bad Gateway") ||
			!request("/b", "group-B/BE-1", "group-B/BE-2") {
			t.Error("failed to receive the right response")
		}
	})
}
