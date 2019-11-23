package kubernetes

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

// TODO:
// - resolve LB backends
// - HTTPS redirect
// - east-west routes
// - catch-all routes
// - review errors and error reporting
// - review and document which errors prevent load and load updates, and which ones are only logged
// - document in the CRD that the service type must be ClusterIP when using service backends

type routeGroups struct{}

type routeGroupContext struct {
	clusterState   *clusterState
	defaultFilters defaultFilters
	routeGroup     *routeGroupItem
	hostRx         string
	backendsByName map[string]*skipperBackend
}

type routeContext struct {
	group      *routeGroupContext
	groupRoute *routeSpec
	id         string
	weight     int
	method     string
	backend    *skipperBackend
}

func newRouteGroups(Options) *routeGroups {
	return &routeGroups{}
}

func invalidBackendRef(rg *routeGroupItem, name string) error {
	return fmt.Errorf(
		"invalid backend reference in routegroup/%s/%s: %s",
		namespaceString(rg.Metadata.Namespace),
		rg.Metadata.Name,
		name,
	)
}

func notSupportedServiceType(s *service) error {
	return fmt.Errorf(
		"not supported service type in service/%s/%s: %s",
		namespaceString(s.Meta.Namespace),
		s.Meta.Name,
		s.Spec.Type,
	)
}

func notImplemented(a ...interface{}) error {
	return fmt.Errorf("not implemented: %v", fmt.Sprint(a...))
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

func crdRouteID(m *metadata, method string, routeIndex, backendIndex int) string {
	return fmt.Sprintf(
		"kube__rg__%s__%s__%s__%d_%d",
		toSymbol(namespaceString(m.Namespace)),
		toSymbol(m.Name),
		toSymbol(method),
		routeIndex,
		backendIndex,
	)
}

func createHostRx(h []string) string {
	if len(h) == 0 {
		return ""
	}

	return fmt.Sprintf("^(%s)$", strings.Join(h, "|"))
}

func mapBackends(backends []*skipperBackend) map[string]*skipperBackend {
	m := make(map[string]*skipperBackend)
	for _, b := range backends {
		m[b.Name] = b
	}

	return m
}

func applyDefaultFilters(ctx *routeGroupContext, serviceName string, r *eskip.Route) error {
	f, err := ctx.defaultFilters.getNamed(ctx.routeGroup.Metadata.Namespace, serviceName)
	if err != nil {
		return err
	}

	// safe to prepend as defaultFilters.get() copies the slice:
	r.Filters = append(f, r.Filters...)
	return nil
}

func getServiceBackend(ctx *routeGroupContext, backend *skipperBackend) (string, error) {
	if backend.ServiceName == "" || backend.ServicePort <= 0 {
		return "", fmt.Errorf(
			"invalid service backend in routegroup/%s/%s: %s:%d",
			namespaceString(ctx.routeGroup.Metadata.Namespace),
			ctx.routeGroup.Metadata.Name,
			backend.ServiceName,
			backend.ServicePort,
		)
	}

	s, err := ctx.clusterState.getService(
		namespaceString(ctx.routeGroup.Metadata.Namespace),
		backend.ServiceName,
	)
	if err != nil {
		return "", err
	}

	if strings.ToLower(s.Spec.Type) != "clusterip" {
		return "", notSupportedServiceType(s)
	}

	var portFound bool
	for _, p := range s.Spec.Ports {
		if p == nil {
			continue
		}

		if p.Port == backend.ServicePort {
			portFound = true
			break
		}
	}

	if !portFound {
		return "", fmt.Errorf(
			"service port not found for routegroup/%s/%s: %d",
			namespaceString(ctx.routeGroup.Metadata.Namespace),
			ctx.routeGroup.Metadata.Name,
			backend.ServicePort,
		)
	}

	return fmt.Sprintf("http://%s:%d", s.Spec.ClusterIP, backend.ServicePort), nil
}

func applyBackend(ctx *routeGroupContext, backend *skipperBackend, r *eskip.Route) error {
	// TODO: resolve to LB with the endpoints
	r.BackendType = backend.Type
	switch r.BackendType {
	case serviceBackend:
		r.BackendType = eskip.NetworkBackend
		var err error
		if r.Backend, err = getServiceBackend(ctx, backend); err != nil {
			return err
		}

		if err := applyDefaultFilters(ctx, backend.ServiceName, r); err != nil {
			log.Errorf("Failed to retrieve default filters: %v.", err)
		}
	case eskip.NetworkBackend:
		r.Backend = backend.Address
	case eskip.LBBackend:
		r.LBAlgorithm = backend.Algorithm.String()
		r.LBEndpoints = backend.Endpoints
	default:
		return notImplemented("backend type", r.BackendType)
	}

	return nil
}

func implicitGroupRoutes(ctx *routeGroupContext) ([]*eskip.Route, error) {
	rg := ctx.routeGroup
	if len(rg.Spec.DefaultBackends) == 0 {
		return nil, fmt.Errorf("missing route spec for route group: %s", rg.Metadata.Name)
	}

	var routes []*eskip.Route
	for backendIndex, beref := range rg.Spec.DefaultBackends {
		if beref == nil {
			log.Errorf(
				"Invalid default backend reference found in: routegroup/%s/%s.",
				namespaceString(rg.Metadata.Namespace),
				rg.Metadata.Name,
			)

			continue
		}

		be, ok := ctx.backendsByName[beref.BackendName]
		if !ok {
			return nil, invalidBackendRef(rg, beref.BackendName)
		}

		rid := crdRouteID(rg.Metadata, "all", 0, backendIndex)
		ri := &eskip.Route{Id: rid}
		if err := applyBackend(ctx, be, ri); err != nil {
			// TODO: log only?
			return nil, err
		}

		if be.Type == serviceBackend {
			if err := applyDefaultFilters(ctx, be.ServiceName, ri); err != nil {
				log.Errorf("Failed to retrieve default filters: %v.", err)
			}
		}

		routes = append(routes, ri)
	}

	if len(rg.Spec.Hosts) > 0 {
		for _, r := range routes {
			r.Predicates = append(r.Predicates, &eskip.Predicate{
				Name: "Host",
				Args: []interface{}{ctx.hostRx},
			})
		}
	}

	return routes, nil
}

func appendPredicate(p []*eskip.Predicate, name string, args ...interface{}) []*eskip.Predicate {
	return append(p, &eskip.Predicate{
		Name: name,
		Args: args,
	})
}

func transformExplicitGroupRoute(ctx *routeContext) (*eskip.Route, error) {
	// TODO: weight

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
			return nil, err
		}

		r.Predicates = append(r.Predicates, ppi...)
	}

	var f []*eskip.Filter
	for _, fi := range gr.Filters {
		ffi, err := eskip.ParseFilters(fi)
		if err != nil {
			return nil, err
		}

		f = append(f, ffi...)
	}

	r.Filters = f
	err := applyBackend(ctx.group, ctx.backend, r)
	return r, err
}

func explicitGroupRoutes(ctx *routeGroupContext) ([]*eskip.Route, error) {
	// TODO: default filters

	var routes []*eskip.Route
	rg := ctx.routeGroup
	for routeIndex, rgr := range rg.Spec.Routes {
		if len(rgr.Methods) == 0 {
			rgr.Methods = []string{""}
		}

		uniqueMethods := make(map[string]struct{})
		for _, m := range rgr.Methods {
			uniqueMethods[m] = struct{}{}
		}

		backendRefs := rg.Spec.DefaultBackends
		if len(rgr.Backends) != 0 {
			backendRefs = rgr.Backends
		}

		// TODO: handling errors. If we consider the route groups independent, then
		// it should be enough to just log them.

		for method := range uniqueMethods {
			for backendIndex, bref := range backendRefs {
				be, ok := ctx.backendsByName[bref.BackendName]
				if !ok {
					return nil, invalidBackendRef(rg, bref.BackendName)
				}

				r, err := transformExplicitGroupRoute(&routeContext{
					group:      ctx,
					groupRoute: rgr,
					id:         crdRouteID(rg.Metadata, method, routeIndex, backendIndex),
					weight:     bref.Weight,
					method:     method,
					backend:    be,
				})
				if err != nil {
					return nil, err
				}

				routes = append(routes, r)
			}
		}
	}

	return routes, nil
}

func transformRouteGroup(ctx *routeGroupContext) ([]*eskip.Route, error) {
	rg := ctx.routeGroup
	if len(rg.Spec.Backends) == 0 {
		return nil, fmt.Errorf("missing backend for route group: %s", rg.Metadata.Name)
	}

	ctx.hostRx = createHostRx(rg.Spec.Hosts)
	ctx.backendsByName = mapBackends(rg.Spec.Backends)
	if len(rg.Spec.Routes) == 0 {
		return implicitGroupRoutes(ctx)
	}

	return explicitGroupRoutes(ctx)
}

func (r *routeGroups) convert(s *clusterState, df defaultFilters) ([]*eskip.Route, error) {
	var rs []*eskip.Route

	var missingName, missingSpec bool
	for _, rg := range s.routeGroups {
		if rg.Metadata == nil || rg.Metadata.Name == "" {
			missingName = true
			continue
		}

		if rg.Spec == nil {
			missingSpec = true
			continue
		}

		ri, err := transformRouteGroup(&routeGroupContext{
			clusterState:   s,
			defaultFilters: df,
			routeGroup:     rg,
		})
		if err != nil {
			log.Errorf("Error transforming route group %s: %v.", rg.Metadata.Name, err)
			continue
		}

		rs = append(rs, ri...)
	}

	if missingName {
		log.Error("One or more route groups without a name were detected.")
	}

	if missingSpec {
		log.Error("One or more route groups without a spec were detected.")
	}

	return rs, nil
}
