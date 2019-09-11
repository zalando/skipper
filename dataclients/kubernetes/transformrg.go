package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/eskip"
)

func createHostRx(h []string) string {
	return fmt.Sprintf("^(%s)$", strings.Join(h, "|"))
}

func mapBackends(backends []*skipperBackend) map[string]*skipperBackend {
	m := make(map[string]*skipperBackend)
	for _, b := range backends {
		m[b.Name] = b
	}
	return m
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
	ns := m.Namespace
	if ns == "" {
		ns = "default"
	}

	return fmt.Sprintf(
		"kube__rg__%s__%s__%s__%d_%d",
		toSymbol(ns),
		toSymbol(m.Name),
		toSymbol(method),
		routeIndex,
		backendIndex,
	)
}

func appendPredicate(p []*eskip.Predicate, name string, args ...interface{}) []*eskip.Predicate {
	return append(p, &eskip.Predicate{
		Name: name,
		Args: args,
	})
}

func invalidBackendRef(name string) error {
	return fmt.Errorf("invalid backend reference: %s", name)
}

func transformRouteGroup(rg *routeGroupItem) ([]*eskip.Route, error) {
	if len(rg.Spec.Backends) == 0 {
		return nil, fmt.Errorf("missing backend for route group: %s", rg.Metadata.Name)
	}

	hostRx := createHostRx(rg.Spec.Hosts)
	refToBackend := mapBackends(rg.Spec.Backends)

	var routes []*eskip.Route
	if len(rg.Spec.Routes) == 0 {
		if len(rg.Spec.DefaultBackends) == 0 {
			return nil, fmt.Errorf("missing route spec for route group: %s", rg.Metadata.Name)
		}

		for i, beref := range rg.Spec.DefaultBackends {
			be, ok := refToBackend[beref.BackendName]
			if !ok {
				return nil, invalidBackendRef(beref.BackendName)
			}

			rid := crdRouteID(rg.Metadata, "all", i, 0)
			ri := &eskip.Route{
				Id:          rid,
				BackendType: be.Type,
				Backend:     be.String(),
				LBAlgorithm: be.Algorithm.String(),
				LBEndpoints: be.Endpoints,
			}

			routes = append(routes, ri)
		}

		if len(rg.Spec.Hosts) > 0 {
			for _, r := range routes {
				r.HostRegexps = []string{hostRx}
			}
		}

		return routes, nil
	}

	for i, sr := range rg.Spec.Routes {
		if len(sr.Methods) == 0 {
			sr.Methods = []string{""}
		}

		uniqueMethods := make(map[string]struct{})
		for _, m := range sr.Methods {
			uniqueMethods[m] = struct{}{}
		}

		for method := range uniqueMethods {
			backendRefs := rg.Spec.DefaultBackends
			if len(sr.Backends) != 0 {
				// case override defaultBackends
				backendRefs = sr.Backends
			}

			for j, bref := range backendRefs {
				be, ok := refToBackend[bref.BackendName]
				if !ok {
					return nil, invalidBackendRef(bref.BackendName)
				}

				r, err := transformRoute(
					sr,
					be,
					crdRouteID(rg.Metadata, method, i, j),
					hostRx,
					method,
					bref.Weight,
				)

				if err != nil {
					return nil, err
				}

				routes = append(routes, r)
			}
		}
	}

	return routes, nil
}

// transformRoute creates one eskip.Route for the specified input
func transformRoute(sr *routeSpec, be *skipperBackend, rid, hostRx, method string, weight int) (*eskip.Route, error) {
	ri := &eskip.Route{Id: rid}

	// Path or PathSubtree, prefer Path if we have, becasuse it is more specifc
	if sr.Path != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "Path", sr.Path)
	} else if sr.PathSubtree != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "PathSubtree", sr.PathSubtree)
	}

	if sr.PathRegexp != "" {
		// TODO: do we need to validate regexp correctness? No, it's compiled in a later phase
		ri.Predicates = appendPredicate(ri.Predicates, "PathRegexp", sr.PathRegexp)
	}

	if hostRx != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "Host", hostRx)
	}

	if method != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "Method", strings.ToUpper(method))
	}

	// handle predicates list
	for _, pi := range sr.Predicates {
		ppi, err := eskip.ParsePredicates(pi)
		if err != nil {
			return nil, err
		}

		ri.Predicates = append(ri.Predicates, ppi...)
	}

	var f []*eskip.Filter
	for _, fi := range sr.Filters {
		ffi, err := eskip.ParseFilters(fi)
		if err != nil {
			return nil, err
		}

		f = append(f, ffi...)
	}

	ri.Filters = f

	t := be.Type
	if t == serviceBackend {
		// TODO: resolve to LB with the endpoints
		t = eskip.NetworkBackend
	}

	ri.BackendType = t
	switch t {
	case eskip.NetworkBackend:
		ri.Backend = be.Address
	case eskip.LBBackend:
		ri.LBAlgorithm = be.Algorithm.String()
		ri.LBEndpoints = be.Endpoints
	}

	return ri, nil
}

func transformRouteGroups(doc []byte) ([]*eskip.Route, error) {
	var rgs routeGroupList
	if err := json.Unmarshal(doc, &rgs); err != nil {
		return nil, err
	}

	var (
		missingName bool
		r           []*eskip.Route
	)

	for _, rg := range rgs.Items {
		if rg == nil || rg.Spec == nil {
			continue
		}

		if rg.Metadata == nil || rg.Metadata.Name == "" {
			missingName = true
			continue
		}

		rgr, err := transformRouteGroup(rg)
		if err != nil {
			log.Errorf("Error transforming route group %s: %v.", rg.Metadata.Name, err)
			continue
		}

		r = append(r, rgr...)
	}

	if missingName {
		log.Error("One or more route groups without name detected.")
	}

	return r, nil
}
