package kubernetes

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
)

const backendNameTracingTagName = "skipper.backend_name"

// TODO:
// - consider catchall for east-west routes

type routeGroups struct {
	options Options
}

type routeGroupContext struct {
	clusterState          *clusterState
	defaultFilters        defaultFilters
	routeGroup            *definitions.RouteGroupItem
	hosts                 []string
	hostRx                string
	hostRoutes            map[string][]*eskip.Route
	hasEastWestHost       bool
	eastWestEnabled       bool
	eastWestDomain        string
	provideHTTPSRedirect  bool
	httpsRedirectCode     int
	backendsByName        map[string]*definitions.SkipperBackend
	defaultBackendTraffic map[string]*calculatedTraffic
	backendNameTracingTag bool
	internal              bool
}

type routeContext struct {
	group      *routeGroupContext
	groupRoute *definitions.RouteSpec
	id         string
	method     string
	backend    *definitions.SkipperBackend
}

type calculatedTraffic struct {
	value   float64
	balance int
}

func eskipError(typ, e string, err error) error {
	if len(e) > 48 {
		e = e[:48]
	}

	return fmt.Errorf("[eskip] %s, '%s'; %v", typ, e, err)
}

func targetPortNotFound(serviceName string, servicePort int) error {
	return fmt.Errorf("target port not found: %s:%d", serviceName, servicePort)
}

func newRouteGroups(o Options) *routeGroups {
	return &routeGroups{options: o}
}

func namespaceString(ns string) string {
	if ns == "" {
		return "default"
	}

	return ns
}

func notSupportedServiceType(s *service) error {
	return fmt.Errorf(
		"not supported service type in service/%s/%s: %s",
		namespaceString(s.Meta.Namespace),
		s.Meta.Name,
		s.Spec.Type,
	)
}

func defaultFiltersError(m *definitions.Metadata, service string, err error) error {
	return fmt.Errorf(
		"error while applying default filters for route group and service: %s/%s %s, %w",
		namespaceString(m.Namespace),
		m.Name,
		service,
		err,
	)
}

func hasEastWestHost(eastWestPostfix string, hosts []string) bool {
	for _, h := range hosts {
		if strings.HasSuffix(h, eastWestPostfix) {
			return true
		}
	}

	return false
}

func toSymbol(p string) string {
	b := []byte(p)
	for i := range b {
		if b[i] == '_' ||
			b[i] >= '0' && b[i] <= '9' ||
			b[i] >= 'a' && b[i] <= 'z' ||
			b[i] >= 'A' && b[i] <= 'Z' {
			continue
		}

		b[i] = '_'
	}

	return string(b)
}

func rgRouteID(namespace, name, subName string, index, subIndex int, internal bool) string {
	if internal {
		namespace = "internal_" + namespace
	}
	return fmt.Sprintf(
		"kube_rg__%s__%s__%s__%d_%d",
		namespace,
		name,
		subName,
		index,
		subIndex,
	)
}

func crdRouteID(m *definitions.Metadata, method string, routeIndex, backendIndex int, internal bool) string {
	return rgRouteID(
		toSymbol(namespaceString(m.Namespace)),
		toSymbol(m.Name),
		toSymbol(method),
		routeIndex,
		backendIndex,
		internal,
	)
}

func mapBackends(backends []*definitions.SkipperBackend) map[string]*definitions.SkipperBackend {
	m := make(map[string]*definitions.SkipperBackend)
	for _, b := range backends {
		m[b.Name] = b
	}

	return m
}

// calculateTraffic calculates the traffic values for the skipper Traffic() predicates
// based on the weight values in the backend references. It represents the remainder
// traffic as 1, where no Traffic predicate is meant to be set.
func calculateTraffic(b []*definitions.BackendReference) map[string]*calculatedTraffic {
	var sum int
	weights := make([]int, len(b))
	for i, bi := range b {
		sum += bi.Weight
		weights[i] = bi.Weight
	}

	if sum == 0 {
		sum = len(weights)
		for i := range weights {
			weights[i] = 1
		}
	}

	var lastWithWeight int
	for i, w := range weights {
		if w > 0 {
			lastWithWeight = i
		}
	}

	t := make(map[string]*calculatedTraffic)
	for i, bi := range b {
		switch {
		case i == lastWithWeight:
			t[bi.BackendName] = &calculatedTraffic{value: 1}
		case weights[i] == 0:
			t[bi.BackendName] = &calculatedTraffic{value: 0}
		default:
			t[bi.BackendName] = &calculatedTraffic{value: float64(weights[i]) / float64(sum)}
		}

		sum -= weights[i]
		t[bi.BackendName].balance = len(b) - i - 2
	}

	return t
}

func trafficBalance(t *calculatedTraffic) []*eskip.Predicate {
	if t.balance <= 0 {
		return nil
	}

	p := eskip.Predicate{Name: "True"}
	balance := make([]*eskip.Predicate, t.balance)
	for i := range balance {
		balance[i] = eskip.CopyPredicate(&p)
	}

	return balance
}

func configureTraffic(r *eskip.Route, t *calculatedTraffic) {
	if t.value == 1 {
		return
	}

	r.Predicates = appendPredicate(r.Predicates, "Traffic", t.value)
	r.Predicates = append(r.Predicates, trafficBalance(t)...)
}

func getBackendService(ctx *routeGroupContext, backend *definitions.SkipperBackend) (*service, error) {
	s, err := ctx.clusterState.getServiceRG(
		namespaceString(ctx.routeGroup.Metadata.Namespace),
		backend.ServiceName,
	)
	if err != nil {
		return nil, err
	}

	if strings.ToLower(s.Spec.Type) != "clusterip" {
		return nil, notSupportedServiceType(s)
	}

	return s, nil
}

func applyServiceBackend(ctx *routeGroupContext, backend *definitions.SkipperBackend, r *eskip.Route) error {
	protocol := "http"
	if p, ok := ctx.routeGroup.Metadata.Annotations[skipperBackendProtocolAnnotationKey]; ok {
		protocol = p
	}

	s, err := getBackendService(ctx, backend)
	if err != nil {
		return err
	}

	targetPort, ok := s.getTargetPortByValue(backend.ServicePort)
	if !ok {
		return targetPortNotFound(backend.ServiceName, backend.ServicePort)
	}

	eps := ctx.clusterState.getEndpointsByTarget(
		namespaceString(ctx.routeGroup.Metadata.Namespace),
		s.Meta.Name,
		protocol,
		targetPort,
	)

	if len(eps) == 0 {
		log.Infof(
			"[routegroup] Target endpoints not found, shuntroute for %s/%s %s:%d",
			namespaceString(ctx.routeGroup.Metadata.Namespace),
			ctx.routeGroup.Metadata.Name,
			backend.ServiceName,
			backend.ServicePort,
		)

		shuntRoute(r)
		return nil
	}

	if len(eps) == 1 {
		r.BackendType = eskip.NetworkBackend
		r.Backend = eps[0]
		return nil
	}

	r.BackendType = eskip.LBBackend
	r.LBEndpoints = eps
	r.LBAlgorithm = defaultLoadBalancerAlgorithm
	if backend.Algorithm != loadbalancer.None {
		r.LBAlgorithm = backend.Algorithm.String()
	}

	return nil
}

func applyDefaultFilters(ctx *routeGroupContext, serviceName string, r *eskip.Route) error {
	f, err := ctx.defaultFilters.getNamed(namespaceString(ctx.routeGroup.Metadata.Namespace), serviceName)
	if err != nil {
		return defaultFiltersError(ctx.routeGroup.Metadata, serviceName, err)
	}

	// safe to prepend as defaultFilters.get() copies the slice:
	r.Filters = append(f, r.Filters...)
	return nil
}

func appendFilter(f []*eskip.Filter, name string, args ...interface{}) []*eskip.Filter {
	return append(f, &eskip.Filter{
		Name: name,
		Args: args,
	})
}

func applyBackend(ctx *routeGroupContext, backend *definitions.SkipperBackend, r *eskip.Route) error {
	r.BackendType = backend.Type
	switch r.BackendType {
	case definitions.ServiceBackend:
		if err := applyServiceBackend(ctx, backend, r); err != nil {
			return err
		}
	case eskip.NetworkBackend:
		r.Backend = backend.Address
	case eskip.LBBackend:
		r.LBEndpoints = backend.Endpoints
		r.LBAlgorithm = defaultLoadBalancerAlgorithm
		if backend.Algorithm != loadbalancer.None {
			r.LBAlgorithm = backend.Algorithm.String()
		}
	}

	if ctx.backendNameTracingTag {
		r.Filters = appendFilter(r.Filters, "tracingTag", backendNameTracingTagName, backend.Name)
	}

	return nil
}

func appendPredicate(p []*eskip.Predicate, name string, args ...interface{}) []*eskip.Predicate {
	return append(p, &eskip.Predicate{
		Name: name,
		Args: args,
	})
}

func storeHostRoute(ctx *routeGroupContext, r *eskip.Route) {
	for _, h := range ctx.hosts {
		ctx.hostRoutes[h] = append(ctx.hostRoutes[h], r)
	}
}

func appendEastWest(ctx *routeGroupContext, routes []*eskip.Route, current *eskip.Route) []*eskip.Route {
	if !ctx.eastWestEnabled || ctx.hasEastWestHost {
		return routes
	}

	ewr := createEastWestRouteRG(
		ctx.routeGroup.Metadata.Name,
		namespaceString(ctx.routeGroup.Metadata.Namespace),
		ctx.eastWestDomain,
		current,
	)

	return append(routes, ewr)
}

func appendHTTPSRedirect(ctx *routeGroupContext, routes []*eskip.Route, current *eskip.Route) []*eskip.Route {
	// in case a route explicitly handles the forwarded proto header, we
	// don't shadow it

	if ctx.provideHTTPSRedirect && !hasProtoPredicate(current) {
		hsr := createHTTPSRedirect(ctx.httpsRedirectCode, current)
		routes = append(routes, hsr)
	}

	return routes
}

// implicitGroupRoutes creates routes for those route groups where the `route`
// field is not defined, and the routes are derived from the default backends.
func implicitGroupRoutes(ctx *routeGroupContext) ([]*eskip.Route, error) {
	rg := ctx.routeGroup

	var routes []*eskip.Route
	for backendIndex, beref := range rg.Spec.DefaultBackends {
		be := ctx.backendsByName[beref.BackendName]
		rid := crdRouteID(rg.Metadata, "all", 0, backendIndex, ctx.internal)
		ri := &eskip.Route{Id: rid}
		if err := applyBackend(ctx, be, ri); err != nil {
			return nil, err
		}

		if ctx.hostRx != "" {
			ri.Predicates = appendPredicate(ri.Predicates, "Host", ctx.hostRx)
		}

		configureTraffic(ri, ctx.defaultBackendTraffic[beref.BackendName])
		if be.Type == definitions.ServiceBackend {
			if err := applyDefaultFilters(ctx, be.ServiceName, ri); err != nil {
				log.Errorf("[routegroup]: failed to retrieve default filters: %v.", err)
			}
		}

		storeHostRoute(ctx, ri)
		routes = append(routes, ri)
		routes = appendEastWest(ctx, routes, ri)
		routes = appendHTTPSRedirect(ctx, routes, ri)
	}

	return routes, nil
}

func transformExplicitGroupRoute(ctx *routeContext) (*eskip.Route, error) {
	gr := ctx.groupRoute
	r := &eskip.Route{Id: ctx.id}

	// Path or PathSubtree, prefer Path if we have, because it is more specifc
	if gr.Path != "" {
		r.Predicates = appendPredicate(r.Predicates, "Path", gr.Path)
	} else if gr.PathSubtree != "" {
		r.Predicates = appendPredicate(r.Predicates, "PathSubtree", gr.PathSubtree)
	}

	if gr.PathRegexp != "" {
		r.Predicates = appendPredicate(r.Predicates, "PathRegexp", gr.PathRegexp)
	}

	if ctx.group.hostRx != "" {
		r.Predicates = appendPredicate(r.Predicates, "Host", ctx.group.hostRx)
	}

	if ctx.method != "" {
		r.Predicates = appendPredicate(r.Predicates, "Method", strings.ToUpper(ctx.method))
	}

	for _, pi := range gr.Predicates {
		ppi, err := eskip.ParsePredicates(pi)
		if err != nil {
			return nil, eskipError("predicate", pi, err)
		}

		r.Predicates = append(r.Predicates, ppi...)
	}

	var f []*eskip.Filter
	for _, fi := range gr.Filters {
		ffi, err := eskip.ParseFilters(fi)
		if err != nil {
			return nil, eskipError("filter", fi, err)
		}

		f = append(f, ffi...)
	}

	r.Filters = f
	err := applyBackend(ctx.group, ctx.backend, r)
	if err != nil {
		return nil, err
	}

	if ctx.backend.Type == definitions.ServiceBackend {
		if err := applyDefaultFilters(ctx.group, ctx.backend.ServiceName, r); err != nil {
			log.Errorf("[routegroup]: failed to retrieve default filters: %v.", err)
		}
	}

	return r, nil
}

// explicitGroupRoutes creates routes for those route groups that have the
// `route` field explicitly defined.
func explicitGroupRoutes(ctx *routeGroupContext) ([]*eskip.Route, error) {
	var routes []*eskip.Route
	rg := ctx.routeGroup
	for routeIndex, rgr := range rg.Spec.Routes {
		if len(rgr.Methods) == 0 {
			rgr.Methods = []string{""}
		}

		backendRefs := rg.Spec.DefaultBackends
		backendTraffic := ctx.defaultBackendTraffic
		if len(rgr.Backends) != 0 {
			backendRefs = rgr.Backends
			backendTraffic = calculateTraffic(rgr.Backends)
		}

		for _, method := range rgr.UniqueMethods() {
			for backendIndex, bref := range backendRefs {
				be := ctx.backendsByName[bref.BackendName]
				idMethod := strings.ToLower(method)
				if idMethod == "" {
					idMethod = "all"
				}

				r, err := transformExplicitGroupRoute(&routeContext{
					group:      ctx,
					groupRoute: rgr,
					id:         crdRouteID(rg.Metadata, idMethod, routeIndex, backendIndex, ctx.internal),
					method:     strings.ToUpper(method),
					backend:    be,
				})
				if err != nil {
					return nil, err
				}

				configureTraffic(r, backendTraffic[bref.BackendName])
				storeHostRoute(ctx, r)
				routes = append(routes, r)
				routes = appendEastWest(ctx, routes, r)
				routes = appendHTTPSRedirect(ctx, routes, r)
			}
		}
	}

	return routes, nil
}

func transformRouteGroup(ctx *routeGroupContext) ([]*eskip.Route, error) {
	ctx.defaultBackendTraffic = calculateTraffic(ctx.routeGroup.Spec.DefaultBackends)
	if len(ctx.routeGroup.Spec.Routes) == 0 {
		return implicitGroupRoutes(ctx)
	}

	return explicitGroupRoutes(ctx)
}

func splitHosts(hosts []string, domains []string) ([]string, []string) {
	internalHosts := []string{}
	externalHosts := []string{}

	for _, host := range hosts {
		for _, d := range domains {
			if strings.HasSuffix(host, d) {
				internalHosts = append(internalHosts, host)
			} else {
				externalHosts = append(externalHosts, host)
			}
		}
	}

	return internalHosts, externalHosts
}

func (r *routeGroups) convert(s *clusterState, df defaultFilters) ([]*eskip.Route, error) {
	var rs []*eskip.Route

	for _, rg := range s.routeGroups {
		var internalHosts []string
		var externalHosts []string

		hosts := rg.Spec.UniqueHosts()
		if len(r.options.KubernetesEastWestRangeDomains) == 0 {
			externalHosts = hosts
		} else {
			internalHosts, externalHosts = splitHosts(hosts, r.options.KubernetesEastWestRangeDomains)
		}

		backends := mapBackends(rg.Spec.Backends)

		// If there's no host at all, or if there's any external hosts
		// create it.
		if len(externalHosts) != 0 || len(hosts) == 0 {
			ctx := &routeGroupContext{
				clusterState:          s,
				defaultFilters:        df,
				routeGroup:            rg,
				hosts:                 externalHosts,
				hostRx:                createHostRx(externalHosts...),
				hostRoutes:            make(map[string][]*eskip.Route),
				hasEastWestHost:       hasEastWestHost(r.options.KubernetesEastWestDomain, externalHosts),
				eastWestEnabled:       r.options.KubernetesEnableEastWest,
				eastWestDomain:        r.options.KubernetesEastWestDomain,
				provideHTTPSRedirect:  r.options.ProvideHTTPSRedirect,
				httpsRedirectCode:     r.options.HTTPSRedirectCode,
				backendsByName:        backends,
				backendNameTracingTag: r.options.BackendNameTracingTag,
				internal:              false,
			}

			ri, err := transformRouteGroup(ctx)
			if err != nil {
				log.Errorf(
					"[routegroup] error transforming external hosts for %s/%s: %v.",
					namespaceString(rg.Metadata.Namespace),
					rg.Metadata.Name,
					err,
				)

				continue
			}

			catchAll := hostCatchAllRoutes(ctx.hostRoutes, func(host string) string {
				// "catchall" won't conflict with any HTTP method
				return rgRouteID("", toSymbol(host), "catchall", 0, 0, false)
			})
			ri = append(ri, catchAll...)

			rs = append(rs, ri...)
		}

		// Internal hosts
		if len(internalHosts) > 0 {
			internalCtx := &routeGroupContext{
				clusterState:          s,
				defaultFilters:        df,
				routeGroup:            rg,
				hosts:                 internalHosts,
				hostRx:                createHostRx(internalHosts...),
				hostRoutes:            make(map[string][]*eskip.Route),
				provideHTTPSRedirect:  r.options.ProvideHTTPSRedirect,
				httpsRedirectCode:     r.options.HTTPSRedirectCode,
				backendsByName:        backends,
				backendNameTracingTag: r.options.BackendNameTracingTag,
				internal:              true,
			}

			internalRi, err := transformRouteGroup(internalCtx)
			if err != nil {
				log.Errorf(
					"[routegroup] error transforming internal hosts for %s/%s: %v.",
					namespaceString(rg.Metadata.Namespace),
					rg.Metadata.Name,
					err,
				)

				continue
			}

			catchAll := hostCatchAllRoutes(internalCtx.hostRoutes, func(host string) string {
				// "catchall" won't conflict with any HTTP method
				return rgRouteID("", toSymbol(host), "catchall", 0, 0, true)
			})
			internalRi = append(internalRi, catchAll...)

			applyEastWestRangePredicates(internalRi, r.options.KubernetesEastWestRangePredicates)

			rs = append(rs, internalRi...)
		}
	}

	return rs, nil
}
