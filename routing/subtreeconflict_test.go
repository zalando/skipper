package routing_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestSubtreeConflict(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const (
		routesOrder1 = `
			subtree: PathSubtree("/foo/") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

			path: Path("/foo")
			  -> status(200)
			  -> <shunt>;
		`

		routesOrder2 = `
			path: Path("/foo")
			  -> status(200)
			  -> <shunt>;

			subtree: PathSubtree("/foo/") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

		`

		routesInvertTrailingOrder1 = `
			subtree: PathSubtree("/foo") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

			path: Path("/foo/")
			  -> status(200)
			  -> <shunt>;
		`

		routesInvertTrailingOrder2 = `
			path: Path("/foo/")
			  -> status(200)
			  -> <shunt>;

			subtree: PathSubtree("/foo") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

		`

		routesNoTrailingOrder1 = `
			subtree: PathSubtree("/foo") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

			path: Path("/foo")
			  -> status(200)
			  -> <shunt>;
		`

		routesNoTrailingOrder2 = `
			path: Path("/foo")
			  -> status(200)
			  -> <shunt>;

			subtree: PathSubtree("/foo") && Method("PUT")
			  -> status(200)
			  -> <shunt>;
		`

		routesWithTrailingOrder1 = `
			subtree: PathSubtree("/foo/") && Method("PUT")
			  -> status(200)
			  -> <shunt>;

			path: Path("/foo/")
			  -> status(200)
			  -> <shunt>;
		`

		routesWithTrailingOrder2 = `
			path: Path("/foo/")
			  -> status(200)
			  -> <shunt>;

			subtree: PathSubtree("/foo/") && Method("PUT")
			  -> status(200)
			  -> <shunt>;
		`
	)

	routingDefs := []struct {
		name    string
		routes  string
		options routing.MatchingOptions
	}{
		{"order 1, keep trailing", routesOrder1, routing.MatchingOptionsNone},
		{"order 2, keep trailing", routesOrder2, routing.MatchingOptionsNone},
		{"order 1, ignore trailing", routesOrder1, routing.IgnoreTrailingSlash},
		{"order 2, ignore trailing", routesOrder2, routing.IgnoreTrailingSlash},

		{"invert trailing, order 1, keep trailing", routesInvertTrailingOrder1, routing.MatchingOptionsNone},
		{"invert trailing, order 2, keep trailing", routesInvertTrailingOrder2, routing.MatchingOptionsNone},
		{"invert trailing, order 1, ignore trailing", routesInvertTrailingOrder1, routing.IgnoreTrailingSlash},
		{"invert trailing, order 2, ignore trailing", routesInvertTrailingOrder2, routing.IgnoreTrailingSlash},

		{"no trailing, order 1, keep trailing", routesNoTrailingOrder1, routing.MatchingOptionsNone},
		{"no trailing, order 2, keep trailing", routesNoTrailingOrder2, routing.MatchingOptionsNone},
		{"no trailing, order 1, ignore trailing", routesNoTrailingOrder1, routing.IgnoreTrailingSlash},
		{"no trailing, order 2, ignore trailing", routesNoTrailingOrder2, routing.IgnoreTrailingSlash},

		{"with trailing, order 1, keep trailing", routesWithTrailingOrder1, routing.MatchingOptionsNone},
		{"with trailing, order 2, keep trailing", routesWithTrailingOrder2, routing.MatchingOptionsNone},
		{"with trailing, order 1, ignore trailing", routesWithTrailingOrder1, routing.IgnoreTrailingSlash},
		{"with trailing, order 2, ignore trailing", routesWithTrailingOrder2, routing.IgnoreTrailingSlash},
	}

	reqs := []struct {
		name          string
		expectedRoute string
		skipRouting   []string
		request       *http.Request
	}{{
		name:          "match subtree, with subpath",
		expectedRoute: "subtree",
		request: &http.Request{
			Method: "PUT",
			URL:    &url.URL{Path: "/foo/bar"},
		},
	}, {
		name:          "match subtree, without subpath",
		expectedRoute: "subtree",
		request: &http.Request{
			Method: "PUT",
			URL:    &url.URL{Path: "/foo"},
		},
	}, {
		name:          "match subtree, with trailing slash",
		expectedRoute: "subtree",
		request: &http.Request{
			Method: "PUT",
			URL:    &url.URL{Path: "/foo/"},
		},
	}, {
		name:          "match exact path",
		expectedRoute: "path",
		skipRouting: []string{
			"with trailing, order 1, keep trailing",
			"with trailing, order 2, keep trailing",
			"invert trailing, order 1, keep trailing",
			"invert trailing, order 2, keep trailing",
		},
		request: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/foo"},
		},
	}, {
		name:          "no match",
		expectedRoute: "",
		request: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/bar"},
		},
	}}

	log := loggingtest.New()
	defer log.Close()

	for _, req := range reqs {
		t.Run(req.name, func(t *testing.T) {
			for _, def := range routingDefs {
				t.Run(def.name, func(t *testing.T) {
					defer log.Reset()

					if slices.Contains(req.skipRouting, def.name) {
						t.Skip()
						return
					}

					dc, err := testdataclient.NewDoc(def.routes)
					if err != nil {
						t.Error(err)
						return
					}
					defer dc.Close()

					r := routing.New(routing.Options{
						FilterRegistry:  builtin.MakeRegistry(),
						DataClients:     []routing.DataClient{dc},
						Log:             log,
						MatchingOptions: def.options,
					})
					defer r.Close()

					if err := log.WaitFor("applied", 12*time.Millisecond); err != nil {
						t.Error(err)
						return
					}

					rt, _ := r.Route(req.request)
					if req.expectedRoute == "" && rt != nil {
						t.Error("unexpectedly matched a route")
						return
					}

					if req.expectedRoute == "" {
						return
					}

					if rt == nil {
						t.Error("failed to match a route")
						return
					}

					if rt.Id != req.expectedRoute {
						t.Error("failed to match the right route")
					}
				})
			}
		})
	}
}
