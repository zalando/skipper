package kubernetes

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
)

// TODO:
// - document how route group errors are handled
// - document in the CRD that the service type must be ClusterIP when using service backends
// - document the implicit routes, or clarify: spec.routes is not optional, but an example doesn't have any
// - document the rules and the loopholes with the host catch-all routes
// - document the behavior of the weight implementation

type routeGroups struct {
	options Options
}

type routeGroupContext struct {
	clusterState          *clusterState
	defaultFilters        defaultFilters
	routeGroup            *routeGroupItem
	hosts                 []string
	hostRx                string
	hostRoutes            map[string][]*eskip.Route
	hasEastWestHost       bool
	eastWestEnabled       bool
	eastWestDomain        string
	provideHTTPSRedirect  bool
	httpsRedirectCode     int
	backendsByName        map[string]*skipperBackend
	defaultBackendTraffic map[string]float64
}

type routeContext struct {
	group      *routeGroupContext
	groupRoute *routeSpec
	id         string
	weight     int
	method     string
	backend    *skipperBackend
}

var errMissingClusterIP = errors.New("missing cluster IP")

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

func notSupportedServiceType(s *service) error {
	return fmt.Errorf(
		"not supported service type in service/%s/%s: %s",
		namespaceString(s.Meta.Namespace),
		s.Meta.Name,
		s.Spec.Type,
	)
}

func servicePortNotFound(m *metadata, b *skipperBackend) error {
	return fmt.Errorf(
		"service port not found for route group backend: %s/%s %s",
		namespaceString(m.Namespace),
		m.Name,
		b.Name,
	)
}

func defaultFiltersError(m *metadata, service string, err error) error {
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

func rgRouteID(namespace, name, subName string, index, subIndex int) string {
	return fmt.Sprintf(
		"kube_rg__%s__%s__%s__%d_%d",
		namespace,
		name,
		subName,
		index,
		subIndex,
	)
}

func crdRouteID(m *metadata, method string, routeIndex, backendIndex int) string {
	return rgRouteID(
		toSymbol(namespaceString(m.Namespace)),
		toSymbol(m.Name),
		toSymbol(method),
		routeIndex,
		backendIndex,
	)
}

func mapBackends(backends []*skipperBackend) map[string]*skipperBackend {
	m := make(map[string]*skipperBackend)
	for _, b := range backends {
		m[b.Name] = b
	}

	return m
}

// calculateTraffic calculates the traffic values for the skipper Traffic() predicates
// based on the weight values in the backend references. It represents the remainder
// traffic as 1, where no Traffic predicate is meant to be set.
func calculateTraffic(b []*backendReference) map[string]float64 {
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

	traffic := make(map[string]float64)
	for i, bi := range b {
		if sum == 0 {
			traffic[bi.BackendName] = 1
			break
		}

		traffic[bi.BackendName] = float64(weights[i]) / float64(sum)
		sum -= weights[i]
	}

	return traffic
}

func getBackendService(ctx *routeGroupContext, backend *skipperBackend) (*service, error) {
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

func createClusterIPBackend(s *service, backend *skipperBackend) (string, error) {
	if s.Spec.ClusterIP == "" {
		return "", errMissingClusterIP
	}

	return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, backend.ServicePort), nil
}

func applyServiceBackend(ctx *routeGroupContext, backend *skipperBackend, r *eskip.Route) error {
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
		targetPort,
	)

	if len(eps) == 0 {
		b, err := createClusterIPBackend(s, backend)
		if err != nil {
			return err
		}

		log.Infof(
			"[routegroup] Target endpoints not found, using service cluster IP as a fallback for %s/%s %s:%d",
			namespaceString(ctx.routeGroup.Metadata.Namespace),
			ctx.routeGroup.Metadata.Name,
			backend.ServiceName,
			backend.ServicePort,
		)

		r.BackendType = eskip.NetworkBackend
		r.Backend = b
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
	f, err := ctx.defaultFilters.getNamed(ctx.routeGroup.Metadata.Namespace, serviceName)
	if err != nil {
		return defaultFiltersError(ctx.routeGroup.Metadata, serviceName, err)
	}

	// safe to prepend as defaultFilters.get() copies the slice:
	r.Filters = append(f, r.Filters...)
	return nil
}

func applyBackend(ctx *routeGroupContext, backend *skipperBackend, r *eskip.Route) error {
	r.BackendType = backend.Type
	switch r.BackendType {
	case serviceBackend:
		if err := applyServiceBackend(ctx, backend, r); err != nil {
			return err
		}
	case eskip.NetworkBackend:
		r.Backend = backend.Address
	case eskip.LBBackend:
		r.LBEndpoints = backend.Endpoints
		r.LBAlgorithm = defaultLoadBalancerAlgorithm
		r.LBAlgorithm = backend.Algorithm.String()
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
	// how will the route group name for the domain name play together with
	// zalando.org/v1/stackset and zalando.org/v1/fabricgateway? Wouldn't it be better to
	// use the service name instead?

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
		rid := crdRouteID(rg.Metadata, "all", 0, backendIndex)
		ri := &eskip.Route{Id: rid}
		if err := applyBackend(ctx, be, ri); err != nil {
			return nil, err
		}

		if ctx.hostRx != "" {
			ri.Predicates = appendPredicate(ri.Predicates, "Host", ctx.hostRx)
		}

		if traffic := ctx.defaultBackendTraffic[beref.BackendName]; traffic < 1 {
			ri.Predicates = appendPredicate(ri.Predicates, "Traffic", traffic)
		}

		if be.Type == serviceBackend {
			if err := applyDefaultFilters(ctx, backend.ServiceName, r); err != nil {
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

	// Path or PathSubtree, prefer Path if we have, becasuse it is more specifc
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

	if be.Type == serviceBackend {
		if err := applyDefaultFilters(ctx, backend.ServiceName, r); err != nil {
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

		for _, method := range rgr.uniqueMethods() {
			for backendIndex, bref := range backendRefs {
				be := ctx.backendsByName[bref.BackendName]
				idMethod := strings.ToLower(method)
				if idMethod == "" {
					idMethod = "all"
				}

				r, err := transformExplicitGroupRoute(&routeContext{
					group:      ctx,
					groupRoute: rgr,
					id:         crdRouteID(rg.Metadata, idMethod, routeIndex, backendIndex),
					weight:     bref.Weight,
					method:     strings.ToUpper(method),
					backend:    be,
				})
				if err != nil {
					return nil, err
				}

				if traffic := backendTraffic[bref.BackendName]; traffic < 1 {
					r.Predicates = appendPredicate(r.Predicates, "Traffic", traffic)
				}

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

func (r *routeGroups) convert(s *clusterState, df defaultFilters) ([]*eskip.Route, error) {
	var rs []*eskip.Route

	hostRoutes := make(map[string][]*eskip.Route)
	for _, rg := range s.routeGroups {
		hosts := rg.Spec.uniqueHosts()
		ctx := &routeGroupContext{
			clusterState:         s,
			defaultFilters:       df,
			routeGroup:           rg,
			hosts:                hosts,
			hostRx:               createHostRx(hosts...),
			hostRoutes:           hostRoutes,
			hasEastWestHost:      hasEastWestHost(r.options.KubernetesEastWestDomain, hosts),
			eastWestEnabled:      r.options.KubernetesEnableEastWest,
			eastWestDomain:       r.options.KubernetesEastWestDomain,
			provideHTTPSRedirect: r.options.ProvideHTTPSRedirect,
			httpsRedirectCode:    r.options.HTTPSRedirectCode,
			backendsByName:       mapBackends(rg.Spec.Backends),
		}

		ri, err := transformRouteGroup(ctx)
		if err != nil {
			log.Errorf(
				"[routegroup] error transforming %s/%s: %v.",
				namespaceString(rg.Metadata.Namespace),
				rg.Metadata.Name,
				err,
			)

			continue
		}

		rs = append(rs, ri...)
	}

	catchAll := hostCatchAllRoutes(hostRoutes, func(host string) string {
		// "catchall" won't conflict with any HTTP method
		return rgRouteID("", toSymbol(host), "catchall", 0, 0)
	})

	rs = append(rs, catchAll...)
	return rs, nil
}
