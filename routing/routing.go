package routing

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"log"
	"net/http"
	"net/url"
)

type MatchingOptions uint

const (
	MatchingOptionsNone MatchingOptions = 0
	IgnoreTrailingSlash MatchingOptions = 1 << iota
)

func (o MatchingOptions) ignoreTrailingSlash() bool {
	return o&IgnoreTrailingSlash > 0
}

type DataUpdate struct {
    UpsertedRoutes []*eskip.Route
    DeletedIds []string
    Reset bool
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
	filterRegistry  filters.Registry
	matchingOptions MatchingOptions
	getMatcher      <-chan *matcher
}

type routeDefs map[string]*eskip.Route

func setRouteDefs(rd routeDefs, rs []*eskip.Route) routeDefs {
    for _, r := range rs {
        rd[r.Id] = r
    }

    return rd
}

func delRouteDefs(rd routeDefs, ids []string) routeDefs {
    for _, id := range ids {
        delete(rd, id)
    }

    return rd
}

func processUpdate(rd routeDefs, u *DataUpdate) routeDefs {
    if u.Reset {
        rd = make(routeDefs)
    } else {
        rd = delRouteDefs(rd, u.DeletedIds)
    }

    return setRouteDefs(rd, u.UpsertedRoutes)
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
			// individual definition errors are logged and ignored here
			log.Println(err)
		}
	}

	return routes
}

func createMatcher(o MatchingOptions, rs []*Route) *matcher {
	m, errs := newMatcher(rs, o)
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
        routes, update := c.Receive()
        all = setRouteDefs(all, routes)

        go func(update <-chan *DataUpdate) {
            for u := range update {
                updates <- u
            }
        }(update)
    }

    for {
        routes := processRoutes(r.filterRegistry, all)
        m := createMatcher(r.matchingOptions, routes)
        out <- m

        u := <-updates
        all = processUpdate(all, u)
    }
}

func New(fr filters.Registry, o MatchingOptions, dc ...DataClient) *Routing {
	r := &Routing{filterRegistry: fr, matchingOptions: o}
	initialMatcher := createMatcher(MatchingOptionsNone, nil)
	matchersIn, matchersOut := feedMatchers(initialMatcher)
	go r.receiveRoutes(dc, matchersIn)
	r.getMatcher = matchersOut
	return r
}

func (r *Routing) Route(req *http.Request) (*Route, map[string]string) {
	m := <-r.getMatcher
	return m.match(req)
}
