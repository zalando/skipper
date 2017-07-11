package routing

import (
	"fmt"
	"net/url"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/predicates"
)

type incomingType uint

const (
	incomingReset incomingType = iota
	incomingUpdate
)

func (it incomingType) String() string {
	switch it {
	case incomingReset:
		return "reset"
	case incomingUpdate:
		return "update"
	default:
		return "unknown"
	}
}

type routeDefs map[string]*eskip.Route

type incomingData struct {
	typ            incomingType
	client         DataClient
	upsertedRoutes []*eskip.Route
	deletedIds     []string
}

func (d *incomingData) log(l logging.Logger) {
	for _, r := range d.upsertedRoutes {
		l.Infof("route settings, %v, route: %v: %v", d.typ, r.Id, r)
	}

	for _, id := range d.deletedIds {
		l.Infof("route settings, %v, deleted id: %v", d.typ, id)
	}
}

// continously receives route definitions from a data client on the the output channel.
// The function does not return unless quit is closed. When started, it request for the
// whole current set of routes, and continues polling for the subsequent updates. When a
// communication error occurs, it re-requests the whole valid set, and continues polling.
// Currently, the routes with the same id coming from different sources are merged in an
// undeterministic way, but this may change in the future.
func receiveFromClient(c DataClient, o Options, out chan<- *incomingData, quit <-chan struct{}) {
	initial := true
	for {
		var (
			routes     []*eskip.Route
			deletedIDs []string
			err        error
		)

		to := o.PollTimeout

		if initial {
			routes, err = c.LoadAll()
		} else {
			routes, deletedIDs, err = c.LoadUpdate()
		}

		switch {
		case err != nil && initial:
			o.Log.Error("error while receiveing initial data;", err)
		case err != nil:
			o.Log.Error("error while receiving update;", err)
			initial = true
			to = 0
		case initial || len(routes) > 0 || len(deletedIDs) > 0:
			initial = false

			var incoming *incomingData
			if initial {
				incoming = &incomingData{incomingReset, c, routes, nil}
			} else {
				incoming = &incomingData{incomingUpdate, c, routes, deletedIDs}
			}

			select {
			case out <- incoming:
			case <-quit:
				return
			}
		}

		select {
		case <-time.After(to):
		case <-quit:
			return
		}
	}
}

// applies incoming route definitions to key/route map, where
// the keys are the route ids.
func applyIncoming(defs routeDefs, d *incomingData) routeDefs {
	if d.typ == incomingReset || defs == nil {
		defs = make(routeDefs)
	}

	if d.typ == incomingUpdate {
		for _, id := range d.deletedIds {
			delete(defs, id)
		}
	}

	if d.typ == incomingReset || d.typ == incomingUpdate {
		for _, def := range d.upsertedRoutes {
			defs[def.Id] = def
		}
	}

	return defs
}

// merges the route definitions from multiple data clients by route id
func mergeDefs(defsByClient map[DataClient]routeDefs) []*eskip.Route {
	mergeById := make(routeDefs)
	for _, defs := range defsByClient {
		for id, def := range defs {
			mergeById[id] = def
		}
	}

	var all []*eskip.Route
	for _, def := range mergeById {
		all = append(all, def)
	}

	return all
}

// receives the initial set of the route definitiosn and their
// updates from multiple data clients, merges them by route id
// and sends the merged route definitions to the output channel.
//
// The active set of routes from last successful update are used until the
// next successful update.
func receiveRouteDefs(o Options, quit <-chan struct{}) <-chan []*eskip.Route {
	in := make(chan *incomingData)
	out := make(chan []*eskip.Route)
	defsByClient := make(map[DataClient]routeDefs)

	for _, c := range o.DataClients {
		go receiveFromClient(c, o, in, quit)
	}

	go func() {
		for {
			var incoming *incomingData
			select {
			case incoming = <-in:
			case <-quit:
				return
			}

			incoming.log(o.Log)
			c := incoming.client
			defsByClient[c] = applyIncoming(defsByClient[c], incoming)

			select {
			case out <- mergeDefs(defsByClient):
			case <-quit:
				return
			}
		}
	}()

	return out
}

// splits the backend address of a route definition into separate
// scheme and host variables.
func splitBackend(r *eskip.Route) (string, string, error) {
	if r.Shunt || r.BackendType == eskip.ShuntBackend || r.BackendType == eskip.LoopBackend {
		return "", "", nil
	}

	bu, err := url.ParseRequestURI(r.Backend)
	if err != nil {
		return "", "", err
	}

	return bu.Scheme, bu.Host, nil
}

// creates a filter instance based on its definition and its
// specification in the filter registry.
func createFilter(fr filters.Registry, def *eskip.Filter) (filters.Filter, error) {
	spec, ok := fr[def.Name]
	if !ok {
		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

// creates filter instances based on their definition
// and the filter registry.
func createFilters(fr filters.Registry, defs []*eskip.Filter) ([]*RouteFilter, error) {
	var fs []*RouteFilter
	for i, def := range defs {
		f, err := createFilter(fr, def)
		if err != nil {
			return nil, err
		}

		fs = append(fs, &RouteFilter{f, def.Name, i})
	}

	return fs, nil
}

// check if a predicate is a distinguished, path tree predicate
func isTreePredicate(name string) bool {
	switch name {
	case PathSubtreeName:
		return true
	case PathName:
		return true
	default:
		return false
	}
}

// initialize predicate instances from their spec with the concrete arguments
func processPredicates(cpm map[string]PredicateSpec, defs []*eskip.Predicate) ([]Predicate, error) {
	cps := make([]Predicate, 0, len(defs))
	for _, def := range defs {
		if isTreePredicate(def.Name) {
			continue
		}

		spec, ok := cpm[def.Name]
		if !ok {
			return nil, fmt.Errorf("predicate not found: '%s'", def.Name)
		}

		cp, err := spec.Create(def.Args)
		if err != nil {
			return nil, err
		}

		cps = append(cps, cp)
	}

	return cps, nil
}

// returns the subtree path if it is a valid definition
func processPathOrSubTree(p *eskip.Predicate) (string, error) {
	if len(p.Args) != 1 {
		return "", predicates.ErrInvalidPredicateParameters
	}

	if s, ok := p.Args[0].(string); ok {
		return s, nil
	}

	return "", predicates.ErrInvalidPredicateParameters
}

func validTreePredicates(predicates []*eskip.Predicate) bool {
	var has bool
	for _, p := range predicates {
		switch p.Name {
		case PathName, PathSubtreeName:
			if has {
				return false
			}
			has = true
		}
	}
	return true
}

// processes path tree relevant predicates
func processTreePredicates(r *Route, predicates []*eskip.Predicate) error {
	// backwards compatibility
	if r.Path != "" {
		predicates = append(predicates, &eskip.Predicate{Name: PathName, Args: []interface{}{r.Path}})
	}

	if !validTreePredicates(predicates) {
		return fmt.Errorf("multiple tree predicates (Path, PathSubtree) in the route: %s", r.Id)
	}

	for _, p := range predicates {
		switch p.Name {
		case PathName:
			path, err := processPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.path = path
		case PathSubtreeName:
			pst, err := processPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.pathSubtree = pst
		}
	}

	return nil
}

// processes a route definition for the routing table
func processRouteDef(cpm map[string]PredicateSpec, fr filters.Registry, def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, def.Filters)
	if err != nil {
		return nil, err
	}

	cps, err := processPredicates(cpm, def.Predicates)
	if err != nil {
		return nil, err
	}

	r := &Route{Route: *def, Scheme: scheme, Host: host, Predicates: cps, Filters: fs}
	if err := processTreePredicates(r, def.Predicates); err != nil {
		return nil, err
	}

	return r, nil
}

// convert a slice of predicate specs to a map keyed by their names
func mapPredicates(cps []PredicateSpec) map[string]PredicateSpec {
	cpm := make(map[string]PredicateSpec)
	for _, cp := range cps {
		cpm[cp.Name()] = cp
	}

	return cpm
}

// processes a set of route definitions for the routing table
func processRouteDefs(o Options, fr filters.Registry, defs []*eskip.Route) (routes []*Route, invalidDefs []*eskip.Route) {
	cpm := mapPredicates(o.Predicates)
	for _, def := range defs {
		route, err := processRouteDef(cpm, fr, def)
		if err == nil {
			routes = append(routes, route)
		} else {
			invalidDefs = append(invalidDefs, def)
			o.Log.Error(err)
		}
	}
	return
}

type routeTable struct {
	m             *matcher
	validRoutes   []*eskip.Route
	invalidRoutes []*eskip.Route
	created       time.Time
}

// receives the next version of the routing table on the output channel,
// when an update is received on one of the data clients.
func receiveRouteMatcher(o Options, out chan<- *routeTable, quit <-chan struct{}) {
	updates := receiveRouteDefs(o, quit)
	var (
		rt           *routeTable
		outRelay     chan<- *routeTable
		updatesRelay <-chan []*eskip.Route
	)
	updatesRelay = updates
	for {
		select {
		case defs := <-updatesRelay:
			o.Log.Info("route settings received")
			routes, invalidRoutes := processRouteDefs(o, o.FilterRegistry, defs)
			m, errs := newMatcher(routes, o.MatchingOptions)

			invalidRouteIds := make(map[string]struct{})
			validRoutes := []*eskip.Route{}

			for _, err := range errs {
				o.Log.Error(err)
				invalidRouteIds[err.Id] = struct{}{}
			}

			for _, r := range routes {
				if _, found := invalidRouteIds[r.Id]; found {
					invalidRoutes = append(invalidRoutes, &r.Route)
				} else {
					validRoutes = append(validRoutes, &r.Route)
				}
			}

			rt = &routeTable{
				m:             m,
				validRoutes:   validRoutes,
				invalidRoutes: invalidRoutes,
				created:       time.Now().UTC(),
			}
			updatesRelay = nil
			outRelay = out
		case outRelay <- rt:
			rt = nil
			updatesRelay = updates
			outRelay = nil
		case <-quit:
			return
		}
	}
}
