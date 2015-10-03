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

type Backend struct {
	Scheme string
	Host   string
	Shunt  bool
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
			matcher, err := r.processData(data)
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
	initialMatcher := r.createMatcher(nil)
	matchersIn, matchersOut := feedMatchers(initialMatcher)
	r.receiveUpdates(dc.Receive(), matchersIn)
	r.getMatcher = matchersOut
	return r
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}

func splitBackend(def *eskip.Route) (string, string, error) {
	if def.Shunt {
		return "", "", nil
	}

	bu, err := url.ParseRequestURI(def.Backend)
	if err != nil {
		return "", "", err
	}

	return bu.Scheme, bu.Host, nil
}

func (r *Routing) createFilter(def *eskip.Filter) (filters.Filter, error) {
	spec, ok := r.filterRegistry[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

func (r *Routing) createFilters(defs []*eskip.Filter) ([]filters.Filter, error) {
	fs := make([]filters.Filter, len(defs))
	for i, fdef := range defs {
		f, err := r.createFilter(fdef)
		if err != nil {
			return nil, err
		}

		fs[i] = f
	}

	return fs, nil
}

func (r *Routing) convertDef(def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := r.createFilters(def.Filters)
	if err != nil {
		return nil, err
	}

	return &Route{*def, scheme, host, fs}, nil
}

func (r *Routing) convertDefs(eskipDefs []*eskip.Route) []*Route {
	routes := []*Route{}
	for _, d := range eskipDefs {
		rd, err := r.convertDef(d)
		if err == nil {
			routes = append(routes, rd)
		} else {
			// idividual definition errors are accepted here
			log.Println(err)
		}
	}

	return routes
}

func (r *Routing) createMatcher(defs []*Route) *matcher {
	m, errs := makeMatcher(defs, r.ignoreTrailingSlash)
	for _, err := range errs {
		// individual matcher entry errors are logged and ignored here
		log.Println(err)
	}

	return m
}

func (r *Routing) processData(data string) (*matcher, error) {
	eskipDefs, err := eskip.Parse(data)
	if err != nil {
		return nil, err
	}

	matcherDefs := r.convertDefs(eskipDefs)
	return r.createMatcher(matcherDefs), nil
}
