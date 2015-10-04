package routing

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"log"
	"net/http"
	"net/url"
)

type DataUpdate struct {
    UpsertedRoutes []*eskip.Route
    DeletedIds []string
}

type DataClient interface {
    Receive() ([]*eskip.Route, <-chan *DataUpdate)
}

type Route struct {
	eskip.Route
	Scheme, Host    string
	Filters []filters.Filter
}

type Routing struct {
	filterRegistry      filters.Registry
	ignoreTrailingSlash bool
	getMatcher          <-chan *matcher
}

type routeDefs map[string]*eskip.Route

func (rd routeDefs) set(rs []*eskip.Route) {
    for _, r := range rs {
        rd[r.Id] = r
    }
}

func (rd routeDefs) del(ids []string) {
    for _, id := range ids {
        delete(rd, id)
    }
}

func (rd routeDefs) processUpdate(u *DataUpdate) {
    rd.del(u.DeletedIds)
    rd.set(u.UpsertedRoutes)
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

func processRoute(fr filters.Registry, def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, def.Filters)
	if err != nil {
		return nil, err
	}

	return &Route{*def, scheme, host, fs}, nil
}

func processRoutes(fr filters.Registry, defs routeDefs) []*Route {
	routes := []*Route{}
	for _, def := range defs {
		route, err := processRoute(fr, def)
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
	m, errs := newMatcher(rs, ignoreTrailingSlash)
	for _, err := range errs {
		// individual matcher entry errors are logged and ignored here
		log.Println(err)
	}

	return m
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

func (r *Routing) receiveRoutes(clients []DataClient, out chan<- *matcher) {
    all := make(routeDefs)
    updates := make(chan *DataUpdate)

    for _, c := range clients {
        // TODO: the failure of one source can block the others this way
        routes, update := c.Receive()
        all.set(routes)

        go func(update <-chan *DataUpdate) {
            for u := range update {
                updates <- u
            }
        }(update)
    }

    for {
        routes := processRoutes(r.filterRegistry, all)
        m := createMatcher(r.ignoreTrailingSlash, routes)
        out <- m

        u := <-updates
        all.processUpdate(u)
    }
}

func New(fr filters.Registry, ignoreTrailingSlash bool, dc ...DataClient) *Routing {
	r := &Routing{filterRegistry: fr, ignoreTrailingSlash: ignoreTrailingSlash}
	initialMatcher := createMatcher(false, nil)
	matchersIn, matchersOut := feedMatchers(initialMatcher)
	go r.receiveRoutes(dc, matchersIn)
	r.getMatcher = matchersOut
	return r
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}
