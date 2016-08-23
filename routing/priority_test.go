package routing_test

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestPriority(t *testing.T) {
	doc := `
		route1: Priority(1) && PathRegexp(/.html$/)
			-> status(200)
			-> setResponseHeader("X-Route", "route1")
			-> <shunt>;

		// normally shadows route1 because it has more predicates on the same path
		// tree leaf
		route2: Priority(0.5) && Method("GET") && Host("www.example.org") && Header("X-Test", "true")
			-> status(200)
			-> setResponseHeader("X-Route", "route2")
			-> <shunt>;

		// normally shadows route2 because it has a path predicate
		route3: Path("/directory/document") && Host("www.example.org") && Header("X-Test", "true")
			-> status(200)
			-> setResponseHeader("X-Route", "route3")
			-> <shunt>;
	`

	r, err := eskip.Parse(doc)
	if err != nil {
		t.Error(err)
		return
	}

	fr := builtin.MakeRegistry()
	p := proxytest.New(fr, r...)
	defer p.Close()

	req := func(method, path string) (string, error) {
		req, err := http.NewRequest(method, p.URL+path, nil)
		if err != nil {
			return "", err
		}

		req.Close = true
		req.Host = "www.example.org"
		req.Header.Set("X-Test", "true")

		rsp, err := (&http.Client{}).Do(req)
		if err != nil {
			return "", err
		}

		defer rsp.Body.Close()

		return rsp.Header.Get("X-Route"), nil
	}

	if hit, err := req("GET", "/directory/document.html"); err != nil || hit != "route1" {
		t.Error("failed to route", hit, err)
	}

	if hit, err := req("GET", "/directory/document"); err != nil || hit != "route2" {
		t.Error("failed to route", hit, err)
	}

	if hit, err := req("POST", "/directory/document"); err != nil || hit != "route3" {
		t.Error("failed to route", hit, err)
	}
}
