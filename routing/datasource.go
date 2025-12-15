package routing

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/predicates"
)

type incomingType uint

const (
	incomingReset incomingType = iota
	incomingUpdate
)

var errInvalidWeightParams = errors.New("invalid argument for the Weight predicate")

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
	var ticker *time.Ticker
	if o.PollTimeout != 0 {
		ticker = time.NewTicker(o.PollTimeout)
	} else {
		ticker = time.NewTicker(time.Millisecond)
	}
	defer ticker.Stop()
	for {
		var (
			routes     []*eskip.Route
			deletedIDs []string
			err        error
		)

		if initial {
			routes, err = c.LoadAll()
		} else {
			routes, deletedIDs, err = c.LoadUpdate()
		}

		switch {
		case err != nil && initial:
			o.Log.Error("error while receiving initial data;", err)
		case err != nil:
			o.Log.Error("error while receiving update;", err)
			initial = true
			continue
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
		case <-ticker.C:
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

type mergedDefs struct {
	routes  []*eskip.Route
	clients map[DataClient]struct{}
}

// merges the route definitions from multiple data clients by route id
func mergeDefs(defsByClient map[DataClient]routeDefs) mergedDefs {
	clients := make(map[DataClient]struct{}, len(defsByClient))
	mergeByID := make(routeDefs)
	for c, defs := range defsByClient {
		clients[c] = struct{}{}
		for id, def := range defs {
			mergeByID[id] = def
		}
	}

	all := make([]*eskip.Route, 0, len(mergeByID))
	for _, def := range mergeByID {
		all = append(all, def)
	}
	return mergedDefs{routes: all, clients: clients}
}

// receives the initial set of the route definitiosn and their
// updates from multiple data clients, merges them by route id
// and sends the merged route definitions to the output channel.
//
// The active set of routes from last successful update are used until the
// next successful update.
func receiveRouteDefs(o Options, quit <-chan struct{}) <-chan mergedDefs {
	in := make(chan *incomingData)
	out := make(chan mergedDefs)
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

// SplitBackend splits the backend address into separate scheme and host variables.
// This function is exported to be used by validation components.
func SplitBackend(backend string, backendType eskip.BackendType, shunt bool) (string, string, error) {
	if backend == "" {
		return "", "", nil
	}
	if shunt || backendType == eskip.ShuntBackend || backendType == eskip.LoopBackend ||
		backendType == eskip.DynamicBackend || backendType == eskip.LBBackend {
		return "", "", nil
	}

	return net.SchemeHost(backend)
}

// creates a filter instance based on its definition and its
// specification in the filter registry.
func createFilter(o *Options, def *eskip.Filter, cpm map[string]PredicateSpec) (filters.Filter, error) {
	spec, ok := o.FilterRegistry[def.Name]
	if !ok {
		if isTreePredicate(def.Name) || def.Name == predicates.HostName || def.Name == predicates.PathRegexpName || def.Name == predicates.MethodName || def.Name == predicates.HeaderName || def.Name == predicates.HeaderRegexpName {
			return nil, fmt.Errorf("%w: trying to use %q as filter, but it is only available as predicate", errUnknownFilter, def.Name)
		}

		if _, ok := cpm[def.Name]; ok {
			return nil, fmt.Errorf("%w: trying to use %q as filter, but it is only available as predicate", errUnknownFilter, def.Name)
		}

		return nil, fmt.Errorf("%w: filter %q not found", errUnknownFilter, def.Name)
	}

	start := time.Now()

	f, err := spec.CreateFilter(def.Args)

	if o.Metrics != nil {
		o.Metrics.MeasureFilterCreate(def.Name, start)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: failed to create filter %q: %w", errInvalidFilterParams, spec.Name(), err)
	}
	return f, nil
}

// creates filter instances based on their definition
// and the filter registry.
func createFilters(o *Options, defs []*eskip.Filter, cpm map[string]PredicateSpec) ([]*RouteFilter, error) {
	fs := make([]*RouteFilter, 0, len(defs))
	for i, def := range defs {
		f, err := createFilter(o, def, cpm)
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

func getFreeStringArgs(count int, p *eskip.Predicate) ([]string, error) {
	if len(p.Args) != count {
		return nil, fmt.Errorf(
			"invalid length of predicate args in %s, %d instead of %d",
			p.Name,
			len(p.Args),
			count,
		)
	}

	a := make([]string, 0, len(p.Args))
	for i := range p.Args {
		s, ok := p.Args[i].(string)
		if !ok {
			return nil, fmt.Errorf("expected argument of type string, %s", p.Name)
		}
		a = append(a, s)
	}
	return a, nil
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
		case predicates.HostName:
			a, err := getFreeStringArgs(1, p)
			if err != nil {
				return nil, err
			}

			c.HostRegexps = append(c.HostRegexps, a[0])
		case predicates.PathRegexpName:
			a, err := getFreeStringArgs(1, p)
			if err != nil {
				return nil, err
			}

			c.PathRegexps = append(c.PathRegexps, a[0])
		case predicates.MethodName:
			a, err := getFreeStringArgs(1, p)
			if err != nil {
				return nil, err
			}

			c.Method = a[0]
		case predicates.HeaderName:
			a, err := getFreeStringArgs(2, p)
			if err != nil {
				return nil, err
			}

			if c.Headers == nil {
				c.Headers = make(map[string]string)
			}

			c.Headers[a[0]] = a[1]
		case predicates.HeaderRegexpName:
			a, err := getFreeStringArgs(2, p)
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

func parseWeightPredicateArgs(args []interface{}) (int, error) {
	if len(args) != 1 {
		return 0, errInvalidWeightParams
	}

	if weight, ok := args[0].(float64); ok {
		return int(weight), nil
	}

	if weight, ok := args[0].(int); ok {
		return weight, nil
	}

	return 0, errInvalidWeightParams
}

// initialize predicate instances from their spec with the concrete arguments
func processPredicates(o *Options, cpm map[string]PredicateSpec, defs []*eskip.Predicate) ([]Predicate, int, error) {
	cps := make([]Predicate, 0, len(defs))
	var weight int
	for _, def := range defs {
		if def.Name == predicates.WeightName {
			var w int
			var err error

			if w, err = parseWeightPredicateArgs(def.Args); err != nil {
				return nil, 0, fmt.Errorf("%w: %w", errInvalidPredicateParams, err)
			}

			weight += w

			continue
		}

		if isTreePredicate(def.Name) {
			continue
		}

		spec, ok := cpm[def.Name]
		if !ok {
			return nil, 0, fmt.Errorf("%w: predicate %q not found", errUnknownPredicate, def.Name)
		}

		cp, err := spec.Create(def.Args)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: failed to create predicate %q: %w", errInvalidPredicateParams, spec.Name(), err)
		}

		if ws, ok := spec.(WeightedPredicateSpec); ok {
			weight += ws.Weight()
		}

		cps = append(cps, cp)
	}

	return cps, weight, nil
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

func validTreePredicates(predicateList []*eskip.Predicate) bool {
	var has bool
	for _, p := range predicateList {
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
func processTreePredicates(r *Route, predicateList []*eskip.Predicate) error {
	// backwards compatibility
	if r.Path != "" {
		predicateList = append(predicateList, &eskip.Predicate{Name: predicates.PathName, Args: []interface{}{r.Path}})
	}

	if !validTreePredicates(predicateList) {
		return fmt.Errorf("multiple tree predicates (Path, PathSubtree) in the route: %s", r.Id)
	}

	for _, p := range predicateList {
		switch p.Name {
		case predicates.PathName:
			path, err := processPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.path = path
		case predicates.PathSubtreeName:
			pst, err := processPathOrSubTree(p)
			if err != nil {
				return err
			}
			r.pathSubtree = pst
		}
	}

	return nil
}

// ValidateRoute processes a route definition for the routing table.
// This function is exported to be used by validation webhooks.
func ValidateRoute(o *Options, def *eskip.Route) (*Route, error) {
	route, err := processRouteDef(o, mapPredicates(o.Predicates), def)
	if err != nil {
		return nil, err
	}

	defer closeFilters(route.Filters)
	return route, nil
}

// processes a route definition for the routing table
func processRouteDef(o *Options, cpm map[string]PredicateSpec, def *eskip.Route) (*Route, error) {
	scheme, host, err := SplitBackend(def.Backend, def.BackendType, def.Shunt)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedBackendSplit, err)
	}

	fs, err := createFilters(o, def.Filters, cpm)
	if err != nil {
		return nil, err
	}

	def, err = mergeLegacyNonTreePredicates(def)
	if err != nil {
		return nil, err
	}

	cps, weight, err := processPredicates(o, cpm, def.Predicates)
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
func mapPredicates(cps []PredicateSpec) map[string]PredicateSpec {
	cpm := make(map[string]PredicateSpec)
	for _, cp := range cps {
		cpm[cp.Name()] = cp
	}

	return cpm
}

// processes a set of route definitions for the routing table
func processRouteDefs(o *Options, defs []*eskip.Route) (routes []*Route, invalidDefs []*eskip.Route) {
	cpm := mapPredicates(o.Predicates)
	for _, def := range defs {
		route, err := processRouteDef(o, cpm, def)
		if err == nil {
			routes = append(routes, route)
		} else {
			invalidDefs = append(invalidDefs, def)
			o.Log.Errorf("failed to process route %s: %v", def.Id, err)

			var defErr invalidDefinitionError
			reason := "other"
			if errors.As(err, &defErr) {
				reason = defErr.Code()
			}

			if o.Metrics != nil {
				o.Metrics.SetInvalidRoute(def.Id, reason)
			}
		}
	}

	return
}

type routeTable struct {
	id            int
	m             *matcher
	once          sync.Once
	routes        []*Route // only used for closing
	validRoutes   []*eskip.Route
	invalidRoutes []*eskip.Route
	clients       map[DataClient]struct{}
	created       time.Time
}

// close routeTable will cleanup all underlying resources, that could
// leak goroutines.
func (rt *routeTable) close() {
	rt.once.Do(func() {
		for _, route := range rt.routes {
			closeFilters(route.Filters)
		}
	})
}

func closeFilters(rfs []*RouteFilter) {
	for _, f := range rfs {
		if fc, ok := f.Filter.(filters.FilterCloser); ok {
			fc.Close()
		}
	}
}

// receives the next version of the routing table on the output channel,
// when an update is received on one of the data clients.
func receiveRouteMatcher(o Options, out chan<- *routeTable, quit <-chan struct{}) {
	updates := receiveRouteDefs(o, quit)
	var (
		rt           *routeTable
		outRelay     chan<- *routeTable
		updatesRelay <-chan mergedDefs
		updateId     int
	)
	updatesRelay = updates
	for {
		select {
		case mdefs := <-updatesRelay:
			updateId++
			start := time.Now()

			o.Log.Infof("route settings received, id: %d", updateId)

			defs := mdefs.routes
			for i := range o.PreProcessors {
				defs = o.PreProcessors[i].Do(defs)
			}

			routes, invalidRoutes := processRouteDefs(&o, defs)

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

			for i := range routes {
				r := routes[i]
				if _, found := invalidRouteIds[r.Id]; found {
					invalidRoutes = append(invalidRoutes, &r.Route)
					// Set individual route metric for matcher errors
					if o.Metrics != nil {
						o.Metrics.SetInvalidRoute(r.Id, errInvalidMatcher.Code())
					}
				} else {
					validRoutes = append(validRoutes, &r.Route)
				}
			}

			sort.SliceStable(validRoutes, func(i, j int) bool {
				return validRoutes[i].Id < validRoutes[j].Id
			})

			rt = &routeTable{
				id:            updateId,
				m:             m,
				routes:        routes,
				validRoutes:   validRoutes,
				invalidRoutes: invalidRoutes,
				clients:       mdefs.clients,
				created:       start,
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
