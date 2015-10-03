package routing

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"log"
	"net/http"
	"net/url"
)

type DataClient interface {
	Receive() <-chan string
}

type Route struct {
	eskip.Route
	Scheme  string
	Host    string
	Filters []filters.Filter
}

type Routing struct {
	filterRegistry      filters.Registry
	ignoreTrailingSlash bool
	getMatcher          <-chan *matcher
}

func splitBackend(r *eskip.Route) (string, string, error) {
	if r.Shunt {
		return "", "", nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return "", "", err
	}

	return bu.Scheme, bu.Host, nil
}

func createFilter(fr filters.Registry, def *eskip.Filter) (filters.Filter, error) {
	spec, ok := fr[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

func createFilters(fr filters.Registry, defs []*eskip.Filter) ([]filters.Filter, error) {
	fs := make([]filters.Filter, len(defs))
	for i, def := range defs {
		f, err := createFilter(fr, def)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

func processRoute(fr filters.Registry, r *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(r)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, r.Filters)
	if err != nil {
		return nil, err
	}

	return &Route{*r, scheme, host, fs}, nil
}

func processRoutes(fr filters.Registry, eskipRoutes []*eskip.Route) []*Route {
	routes := []*Route{}
	for _, eskipRoute := range eskipRoutes {
		route, err := processRoute(fr, eskipRoute)
		if err == nil {
			routes = append(routes, route)
		} else {
			// idividual definition errors are accepted here
			log.Println(err)
		}
	}

	return routes
}

func createMatcher(ignoreTrailingSlash bool, rs []*Route) *matcher {
	m, errs := makeMatcher(rs, ignoreTrailingSlash)
	for _, err := range errs {
		// individual matcher entry errors are logged and ignored here
		log.Println(err)
	}

	return m
}

func processData(fr filters.Registry, ignoreTrailingSlash bool, data string) (*matcher, error) {
	eskipRoutes, err := eskip.Parse(data)
	if err != nil {
		return nil, err
	}

	routes := processRoutes(fr, eskipRoutes)
	return createMatcher(ignoreTrailingSlash, routes), nil
}

func feedMatchers(current *matcher) (chan<- *matcher, <-chan *matcher) {
	// todo: measure impact of buffered channel here for out
	in := make(chan *matcher)
	out := make(chan *matcher)

	go func() {
		for {
			select {
			case current = <-in:
			case out <- current:
			}
		}
	}()

	return in, out
}

func (r *Routing) receiveUpdates(in <-chan string, out chan<- *matcher) {
	go func() {
		for {
			data := <-in
			matcher, err := processData(r.filterRegistry, r.ignoreTrailingSlash, data)
			if err != nil {
				// only logging errors here, and waiting for settings from external
				// sources to be fixed
				log.Println("error while processing route data", err)
				continue
			}

			out <- matcher
		}
	}()
}

func New(dc DataClient, fr filters.Registry, ignoreTrailingSlash bool) *Routing {
	r := &Routing{filterRegistry: fr, ignoreTrailingSlash: ignoreTrailingSlash}
	initialMatcher := createMatcher(false, nil)
	matchersIn, matchersOut := feedMatchers(initialMatcher)
	r.receiveUpdates(dc.Receive(), matchersIn)
	r.getMatcher = matchersOut
	return r
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}
