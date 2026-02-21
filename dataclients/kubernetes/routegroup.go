package kubernetes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/secrets/certregistry"
)

const backendNameTracingTagName = "skipper.backend_name"

// TODO:
// - consider catchall for east-west routes

type routeGroups struct {
	options Options
}

type routeGroupContext struct {
	state                        *clusterState
	routeGroup                   *definitions.RouteGroupItem
	logger                       *logger
	hosts                        []string
	allowedExternalNames         []*regexp.Regexp
	hostRx                       string
	eastWestDomain               string
	hostRoutes                   map[string][]*eskip.Route
	defaultBackendTraffic        map[string]backendTraffic
	defaultFilters               defaultFilters
	httpsRedirectCode            int
	backendsByName               map[string]*definitions.SkipperBackend
	eastWestEnabled              bool
	hasEastWestHost              bool
	backendNameTracingTag        bool
	internal                     bool
	provideHTTPSRedirect         bool
	calculateTraffic             func([]*definitions.BackendReference) map[string]backendTraffic
	defaultLoadBalancerAlgorithm string
	forwardBackendURL            string
	certificateRegistry          *certregistry.CertRegistry
	zone                         string
}

type routeContext struct {
	group      *routeGroupContext
	groupRoute *definitions.RouteSpec
	id         string
	method     string
	backend    *definitions.SkipperBackend
}

func eskipError(typ, e string, err error) error {
	if len(e) > 48 {
		e = e[:48]
	}

	return fmt.Errorf("[eskip] %s, '%s'; %w", typ, e, err)
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

func getBackendService(ctx *routeGroupContext, backend *definitions.SkipperBackend) (*service, error) {
	s, err := ctx.state.getServiceRG(
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

	eps := ctx.state.GetEndpointsByTarget(
		ctx.zone,
		namespaceString(ctx.routeGroup.Metadata.Namespace),
		s.Meta.Name,
		"TCP",
		protocol,
		targetPort,
	)

	if len(eps) == 0 {
		ctx.logger.Tracef("Target endpoints not found, shuntroute for %s:%d", backend.ServiceName, backend.ServicePort)

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
	r.LBAlgorithm = ctx.defaultLoadBalancerAlgorithm
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

func appendFilter(f []*eskip.Filter, name string, args ...any) []*eskip.Filter {
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
		if !isExternalAddressAllowed(ctx.allowedExternalNames, backend.Address) {
			return fmt.Errorf(
				"routegroup with not allowed network backend: %s",
				backend.Address,
			)
		}

		r.Backend = backend.Address
	case eskip.LBBackend:
		for _, ep := range backend.Endpoints {
			if !isExternalAddressAllowed(ctx.allowedExternalNames, ep) {
				return fmt.Errorf(
					"routegroup with not allowed explicit LB endpoint: %s",
					ep,
				)
			}
		}

		r.LBEndpoints = backend.Endpoints
		r.LBAlgorithm = ctx.defaultLoadBalancerAlgorithm
		if backend.Algorithm != loadbalancer.None {
			r.LBAlgorithm = backend.Algorithm.String()
		}
	case eskip.ForwardBackend:
		r.Backend = ctx.forwardBackendURL
		r.BackendType = eskip.NetworkBackend
	}

	if ctx.backendNameTracingTag {
		r.Filters = appendFilter(r.Filters, "tracingTag", backendNameTracingTagName, backend.Name)
	}

	return nil
}

func appendPredicate(p []*eskip.Predicate, name string, args ...any) []*eskip.Predicate {
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
	// when a route explicitly handles the forwarded proto header, we
	// don't shadow it

	if !ctx.internal && ctx.provideHTTPSRedirect && !hasProtoPredicate(current) {
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

		ctx.defaultBackendTraffic[beref.BackendName].apply(ri)
		if be.Type == definitions.ServiceBackend {
			if err := applyDefaultFilters(ctx, be.ServiceName, ri); err != nil {
				ctx.logger.Errorf("Failed to retrieve default filters: %v", err)
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

	// Path or PathSubtree, prefer Path if we have, because it is more specific
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
			ctx.group.logger.Errorf("Failed to retrieve default filters: %v", err)
		}
	}

	return r, nil
}

// explicitGroupRoutes creates routes for those route groups that have the
// `route` field explicitly defined.
func explicitGroupRoutes(ctx *routeGroupContext) ([]*eskip.Route, error) {
	var result []*eskip.Route
	rg := ctx.routeGroup

nextRoute:
	for routeIndex, rgr := range rg.Spec.Routes {
		var routes []*eskip.Route

		if len(rgr.Methods) == 0 {
			rgr.Methods = []string{""}
		}

		backendRefs := rg.Spec.DefaultBackends
		backendTraffic := ctx.defaultBackendTraffic
		if len(rgr.Backends) != 0 {
			backendRefs = rgr.Backends
			backendTraffic = ctx.calculateTraffic(rgr.Backends)
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
					ctx.logger.Errorf("Ignoring route: %v", err)
					continue nextRoute
				}

				backendTraffic[bref.BackendName].apply(r)
				storeHostRoute(ctx, r)
				routes = append(routes, r)
				routes = appendEastWest(ctx, routes, r)
				routes = appendHTTPSRedirect(ctx, routes, r)
			}
		}

		result = append(result, routes...)
	}

	return result, nil
}

func transformRouteGroup(ctx *routeGroupContext) ([]*eskip.Route, error) {
	ctx.defaultBackendTraffic = ctx.calculateTraffic(ctx.routeGroup.Spec.DefaultBackends)
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

// addRouteGroupTLS compares the RouteGroup host list and the RouteGroup TLS host list
// and adds the TLS secret to the registry if a match is found.
func (r *routeGroups) addRouteGroupTLS(ctx *routeGroupContext, tls *definitions.RouteTLSSpec) {
	// Host in the tls section need to explicitly match the host in the RouteGroup
	hostlist := compareStringList(tls.Hosts, ctx.routeGroup.Spec.UniqueHosts())
	if len(hostlist) == 0 {
		ctx.logger.Errorf("No matching tls hosts found - tls hosts: %s, routegroup hosts: %s", tls.Hosts, ctx.routeGroup.Spec.UniqueHosts())
		return
	} else if len(hostlist) != len(tls.Hosts) {
		ctx.logger.Infof("Hosts in TLS and RouteGroup don't match: tls hosts: %s, routegroup hosts: %s", tls.Hosts, ctx.routeGroup.Spec.UniqueHosts())
	}

	// Skip adding certs to registry since no certs defined
	if tls.SecretName == "" {
		ctx.logger.Debugf("No tls secret defined for hosts - %s", tls.Hosts)
		return
	}

	// Secrets should always reside in the same namespace as the RouteGroup
	secretID := definitions.ResourceID{Name: tls.SecretName, Namespace: ctx.routeGroup.Metadata.Namespace}
	secret, ok := ctx.state.secrets[secretID]
	if !ok {
		ctx.logger.Errorf("Failed to find secret %s in namespace %s", secretID.Name, secretID.Namespace)
		return
	}
	addTLSCertToRegistry(ctx.certificateRegistry, ctx.logger, hostlist, secret)

}

func (r *routeGroups) convert(s *clusterState, df defaultFilters, loggingEnabled bool, cr *certregistry.CertRegistry) ([]*eskip.Route, error) {
	var rs []*eskip.Route
	redirect := createRedirectInfo(r.options.ProvideHTTPSRedirect, r.options.HTTPSRedirectCode)

	for _, rg := range s.routeGroups {
		logger := newLogger("RouteGroup", rg.Metadata.Namespace, rg.Metadata.Name, loggingEnabled)

		redirect.initCurrent(rg.Metadata)

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
			var provideRedirect bool
			switch {
			case redirect.enable:
				provideRedirect = true
			case redirect.disable:
				provideRedirect = false
			case redirect.defaultEnabled:
				provideRedirect = true
			}
			ctx := &routeGroupContext{
				state:                        s,
				routeGroup:                   rg,
				logger:                       logger,
				defaultFilters:               df,
				hosts:                        externalHosts,
				hostRx:                       createHostRx(externalHosts...),
				hostRoutes:                   make(map[string][]*eskip.Route),
				hasEastWestHost:              hasEastWestHost(r.options.KubernetesEastWestDomain, externalHosts),
				eastWestEnabled:              r.options.KubernetesEnableEastWest,
				eastWestDomain:               r.options.KubernetesEastWestDomain,
				provideHTTPSRedirect:         provideRedirect,
				httpsRedirectCode:            r.options.HTTPSRedirectCode,
				backendsByName:               backends,
				backendNameTracingTag:        r.options.BackendNameTracingTag,
				internal:                     false,
				allowedExternalNames:         r.options.AllowedExternalNames,
				calculateTraffic:             getBackendTrafficCalculator[*definitions.BackendReference](r.options.BackendTrafficAlgorithm),
				defaultLoadBalancerAlgorithm: r.options.DefaultLoadBalancerAlgorithm,
				forwardBackendURL:            r.options.ForwardBackendURL,
				certificateRegistry:          cr,
				zone:                         r.options.TopologyZone,
			}

			ri, err := transformRouteGroup(ctx)
			if err != nil {
				ctx.logger.Errorf("Error transforming external hosts: %v", err)
				continue
			}

			if !r.options.DisableCatchAllRoutes {
				catchAll := hostCatchAllRoutes(ctx.hostRoutes, func(host string) string {
					// "catchall" won't conflict with any HTTP method
					return rgRouteID("", toSymbol(host), "catchall", 0, 0, false)
				})
				ri = append(ri, catchAll...)
			}

			if ctx.certificateRegistry != nil {
				for _, ctxTls := range rg.Spec.TLS {
					r.addRouteGroupTLS(ctx, ctxTls)
				}
			}

			for _, route := range ri {
				appendAnnotationPredicates(r.options.KubernetesAnnotationPredicates, rg.Metadata.Annotations, route)
				appendAnnotationFilters(r.options.KubernetesAnnotationFiltersAppend, rg.Metadata.Annotations, route)
			}

			rs = append(rs, ri...)
		}

		// Internal hosts
		if len(internalHosts) > 0 {
			internalCtx := &routeGroupContext{
				state:                        s,
				routeGroup:                   rg,
				logger:                       logger,
				defaultFilters:               df,
				hosts:                        internalHosts,
				hostRx:                       createHostRx(internalHosts...),
				hostRoutes:                   make(map[string][]*eskip.Route),
				backendsByName:               backends,
				backendNameTracingTag:        r.options.BackendNameTracingTag,
				internal:                     true,
				allowedExternalNames:         r.options.AllowedExternalNames,
				calculateTraffic:             getBackendTrafficCalculator[*definitions.BackendReference](r.options.BackendTrafficAlgorithm),
				defaultLoadBalancerAlgorithm: r.options.DefaultLoadBalancerAlgorithm,
				forwardBackendURL:            r.options.ForwardBackendURL,
				certificateRegistry:          cr,
			}

			internalRi, err := transformRouteGroup(internalCtx)
			if err != nil {
				internalCtx.logger.Errorf("Error transforming internal hosts: %v", err)

				continue
			}

			if !r.options.DisableCatchAllRoutes {
				catchAll := hostCatchAllRoutes(internalCtx.hostRoutes, func(host string) string {
					// "catchall" won't conflict with any HTTP method
					return rgRouteID("", toSymbol(host), "catchall", 0, 0, true)
				})
				internalRi = append(internalRi, catchAll...)
			}

			applyEastWestRangePredicates(internalRi, r.options.KubernetesEastWestRangePredicates)
			for _, route := range internalRi {
				appendAnnotationPredicates(r.options.KubernetesEastWestRangeAnnotationPredicates, rg.Metadata.Annotations, route)
				appendAnnotationFilters(r.options.KubernetesEastWestRangeAnnotationFiltersAppend, rg.Metadata.Annotations, route)
			}

			if internalCtx.certificateRegistry != nil {
				for _, ctxTls := range rg.Spec.TLS {
					r.addRouteGroupTLS(internalCtx, ctxTls)
				}
			}

			rs = append(rs, internalRi...)
		}
	}

	return rs, nil
}
