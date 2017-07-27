package routing

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
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
		options MatchingOptions
	}{
		{"order 1, keep trailing", routesOrder1, MatchingOptionsNone},
		{"order 2, keep trailing", routesOrder2, MatchingOptionsNone},
		{"order 1, ignore trailing", routesOrder1, IgnoreTrailingSlash},
		{"order 2, ignore trailing", routesOrder2, IgnoreTrailingSlash},

		{"invert trailing, order 1, keep trailing", routesInvertTrailingOrder1, MatchingOptionsNone},
		{"invert trailing, order 2, keep trailing", routesInvertTrailingOrder2, MatchingOptionsNone},
		{"invert trailing, order 1, ignore trailing", routesInvertTrailingOrder1, IgnoreTrailingSlash},
		{"invert trailing, order 2, ignore trailing", routesInvertTrailingOrder2, IgnoreTrailingSlash},

		{"no trailing, order 1, keep trailing", routesNoTrailingOrder1, MatchingOptionsNone},
		{"no trailing, order 2, keep trailing", routesNoTrailingOrder2, MatchingOptionsNone},
		{"no trailing, order 1, ignore trailing", routesNoTrailingOrder1, IgnoreTrailingSlash},
		{"no trailing, order 2, ignore trailing", routesNoTrailingOrder2, IgnoreTrailingSlash},

		{"with trailing, order 1, keep trailing", routesWithTrailingOrder1, MatchingOptionsNone},
		{"with trailing, order 2, keep trailing", routesWithTrailingOrder2, MatchingOptionsNone},
		{"with trailing, order 1, ignore trailing", routesWithTrailingOrder1, IgnoreTrailingSlash},
		{"with trailing, order 2, ignore trailing", routesWithTrailingOrder2, IgnoreTrailingSlash},
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

					for _, skip := range req.skipRouting {
						if skip == def.name {
							t.Skip()
							return
						}
					}

					dc, err := testdataclient.NewDoc(def.routes)
					if err != nil {
						t.Error(err)
						return
					}

					r := New(Options{
						FilterRegistry:  builtin.MakeRegistry(),
						DataClients:     []DataClient{dc},
						Log:             log,
						MatchingOptions: def.options,
					})

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
