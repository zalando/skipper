package routing

import (
	"fmt"
	"github.com/zalando/skipper/predicates"
	"net/url"
	"sort"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
	corepredicates "github.com/zalando/skipper/predicates/core"
	weightpredicate "github.com/zalando/skipper/predicates/weight"
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

func (d *incomingData) log(l logging.Logger, suppress bool) {
	if suppress {
		l.Infof("route settings, %v, upsert count: %v", d.typ, len(d.upsertedRoutes))
		l.Infof("route settings, %v, delete count: %v", d.typ, len(d.deletedIds))
		return
	}

	for _, r := range d.upsertedRoutes {
		l.Infof("route settings, %v, route: %v: %v", d.typ, r.Id, r)
	}

	for _, id := range d.deletedIds {
		l.Infof("route settings, %v, deleted id: %v", d.typ, id)
	}
}

// continuously receives route definitions from a data client on the the output channel.
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
			var incoming *incomingData
			if initial {
				incoming = &incomingData{incomingReset, c, routes, nil}
			} else {
				incoming = &incomingData{incomingUpdate, c, routes, deletedIDs}
			}

			initial = false
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
	mergeByID := make(routeDefs)
	for _, defs := range defsByClient {
		for id, def := range defs {
			mergeByID[id] = def
		}
	}

	var all []*eskip.Route
	for _, def := range mergeByID {
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

			incoming.log(o.Log, o.SuppressLogs)
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
	if r.Shunt || r.BackendType == eskip.ShuntBackend || r.BackendType == eskip.LoopBackend ||
		r.BackendType == eskip.DynamicBackend || r.BackendType == eskip.LBBackend {
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
func createFilter(fr filters.Registry, def *eskip.Filter, cpm map[string]predicates.PredicateSpec) (filters.Filter, error) {
	spec, ok := fr[def.Name]
	if !ok {
		if isTreePredicate(def.Name) || def.Name == predicates.HostRegexpName || def.Name == predicates.PathRegexpName || def.Name == predicates.MethodName || def.Name == predicates.HeaderName || def.Name == predicates.HeaderRegexpName {
			return nil, fmt.Errorf("trying to use '%s' as filter, but it is only available as predicate", def.Name)
		}

		if _, ok := cpm[def.Name]; ok {
			return nil, fmt.Errorf("trying to use '%s' as filter, but it is only available as predicate", def.Name)
		}

		return nil, fmt.Errorf("filter not found: '%s'", def.Name)
	}

	return spec.CreateFilter(def.Args)
}

// creates filter instances based on their definition
// and the filter registry.
func createFilters(fr filters.Registry, defs []*eskip.Filter, cpm map[string]predicates.PredicateSpec) ([]*RouteFilter, error) {
	var fs []*RouteFilter
	for i, def := range defs {
		f, err := createFilter(fr, def, cpm)
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
	case predicates.PathSubtreeName:
		return true
	case predicates.PathName:
		return true
	default:
		return false
	}
}

func mergeLegacyNonTreePredicates(r *eskip.Route) (*eskip.Route, error) {
	var rest []*eskip.Predicate
	c := r.Copy()

	for _, p := range c.Predicates {
		if isTreePredicate(p.Name) {
			rest = append(rest, p)
			continue
		}

		switch p.Name {
		case predicates.HostRegexpName:
			a, err := corepredicates.ValidateHostRegexpPredicate(p)
			if err != nil {
				return nil, err
			}

			c.HostRegexps = append(c.HostRegexps, a[0])
		case predicates.PathRegexpName:
			a, err := corepredicates.ValidatePathRegexpPredicate(p)
			if err != nil {
				return nil, err
			}

			c.PathRegexps = append(c.PathRegexps, a[0])
		case predicates.MethodName:
			a, err := corepredicates.ValidateMethodPredicate(p)
			if err != nil {
				return nil, err
			}

			c.Method = a[0]
		case predicates.HeaderName:
			a, err := corepredicates.ValidateHeaderPredicate(p)
			if err != nil {
				return nil, err
			}

			if c.Headers == nil {
				c.Headers = make(map[string]string)
			}

			c.Headers[a[0]] = a[1]
		case predicates.HeaderRegexpName:
			a, err := corepredicates.ValidateHeaderRegexpPredicate(p)
			if err != nil {
				return nil, err
			}

			if c.HeaderRegexps == nil {
				c.HeaderRegexps = make(map[string][]string)
			}

			c.HeaderRegexps[a[0]] = append(c.HeaderRegexps[a[0]], a[1])
		default:
			rest = append(rest, p)
		}
	}

	c.Predicates = rest
	return c, nil
}

// initialize predicate instances from their spec with the concrete arguments
func processPredicates(cpm map[string]predicates.PredicateSpec, defs []*eskip.Predicate) ([]predicates.Predicate, int, error) {
	cps := make([]predicates.Predicate, 0, len(defs))
	var weight int
	for _, def := range defs {
		if def.Name == predicates.WeightName {
			var err error
			if weight, err = weightpredicate.ParseWeightPredicateArgs(def.Args); err != nil {
				return nil, 0, err
			}

			continue
		}

		if isTreePredicate(def.Name) {
			continue
		}

		spec, ok := cpm[def.Name]
		if !ok {
			return nil, 0, fmt.Errorf("predicate not found: '%s'", def.Name)
		}

		cp, err := spec.Create(def.Args)
		if err != nil {
			return nil, 0, err
		}

		cps = append(cps, cp)
	}

	return cps, weight, nil
}

func validTreePredicates(preds []*eskip.Predicate) bool {
	var has bool
	for _, p := range preds {
		switch p.Name {
		case predicates.PathName, predicates.PathSubtreeName:
			if has {
				return false
			}
			has = true
		}
	}
	return true
}

// processes path tree relevant predicates
func processTreePredicates(r *Route, preds []*eskip.Predicate) error {
	// backwards compatibility
	if r.Path != "" {
		preds = append(preds, &eskip.Predicate{Name: predicates.PathName, Args: []interface{}{r.Path}})
	}

	if !validTreePredicates(preds) {
		return fmt.Errorf("multiple tree predicates (Path, PathSubtree) in the route: %s", r.Id)
	}

	for _, p := range preds {
		switch p.Name {
		case predicates.PathName:
			path, err := corepredicates.ProcessPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.path = path
		case predicates.PathSubtreeName:
			pst, err := corepredicates.ProcessPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.pathSubtree = pst
		}
	}

	return nil
}

// processes a route definition for the routing table
func processRouteDef(cpm map[string]predicates.PredicateSpec, fr filters.Registry, def *eskip.Route) (*Route, error) {
	scheme, host, err := splitBackend(def)
	if err != nil {
		return nil, err
	}

	fs, err := createFilters(fr, def.Filters, cpm)
	if err != nil {
		return nil, err
	}

	def, err = mergeLegacyNonTreePredicates(def)
	if err != nil {
		return nil, err
	}

	cps, weight, err := processPredicates(cpm, def.Predicates)
	if err != nil {
		return nil, err
	}

	r := &Route{Route: *def, Scheme: scheme, Host: host, Predicates: cps, Filters: fs, weight: weight}
	if err := processTreePredicates(r, def.Predicates); err != nil {
		return nil, err
	}

	return r, nil
}

// convert a slice of predicate specs to a map keyed by their names
func mapPredicates(cps []predicates.PredicateSpec) map[string]predicates.PredicateSpec {
	cpm := make(map[string]predicates.PredicateSpec)
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
			o.Log.Errorf("failed to process route (%v): %v", def.Id, err)
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

			for i := range o.PreProcessors {
				defs = o.PreProcessors[i].Do(defs)
			}

			routes, invalidRoutes := processRouteDefs(o, o.FilterRegistry, defs)

			for i := range o.PostProcessors {
				routes = o.PostProcessors[i].Do(routes)
			}

			m, errs := newMatcher(routes, o.MatchingOptions)

			invalidRouteIds := make(map[string]struct{})
			validRoutes := []*eskip.Route{}

			for _, err := range errs {
				o.Log.Error(err)
				invalidRouteIds[err.ID] = struct{}{}
			}

			for _, r := range routes {
				if _, found := invalidRouteIds[r.Id]; found {
					invalidRoutes = append(invalidRoutes, &r.Route)
				} else {
					validRoutes = append(validRoutes, &r.Route)
				}
			}

			sort.SliceStable(validRoutes, func(i, j int) bool {
				return validRoutes[i].Id < validRoutes[j].Id
			})

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
