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

func mapBackends(spec *routeGroupSpec) map[string]*skipperBackend {
	m := make(map[string]*skipperBackend)
	for _, b := range spec.Backends {
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

func crdRouteID(m *metadata, path string, index int) string {
	ns := m.Namespace
	if ns == "" {
		ns = "default"
	}

	return fmt.Sprintf(
		"kube__rg__%s__%s__%s__%d",
		toSymbol(ns),
		toSymbol(m.Name),
		toSymbol(path),
		index,
	)
}

func appendPredicate(p []*eskip.Predicate, name string, args ...interface{}) []*eskip.Predicate {
	return append(p, &eskip.Predicate{
		Name: name,
		Args: args,
	})
}

func transformRouteGroup(rg *routeGroupItem) ([]*eskip.Route, error) {
	if len(rg.Spec.Backends) == 0 {
		return nil, fmt.Errorf("missing backend for route group: %s", rg.Metadata.Name)
	}

	if len(rg.Spec.Routes) == 0 {
		// TODO(sszuecs): should return catchall route, depends on Hosts
		return nil, fmt.Errorf("missing path spec for route group: %s", rg.Metadata.Name)
	}

	hostRx := createHostRx(rg.Spec.Hosts)
	refToBackend := mapBackends(rg.Spec)

	var routes []*eskip.Route
	for i, sr := range rg.Spec.Routes {
		if len(sr.Methods) == 0 {
			sr.Methods = []string{""}
		}
		for _, method := range sr.Methods {

			if len(sr.Backends) != 0 {
				// case override defaultBackends
				for _, bref := range sr.Backends {

					if r, err := getRoute(
						refToBackend,
						sr,
						// TODO: crdRouteID needs more input
						crdRouteID(rg.Metadata, sr.Path, i),
						bref.BackendName,
						hostRx,
						method,
						bref.Weight,
					); err != nil {
						return nil, err
					} else {
						routes = append(routes, r)
					}
				}
			}
		}
	}

	return routes, nil
}

func getRoute(refToBackend map[string]*skipperBackend, sr *routeSpec, rid, beName, hostRx, method string, weight int) (*eskip.Route, error) {
	if be, ok := refToBackend[beName]; ok {
		r, err := transformRoute(
			sr,
			be,
			rid,
			hostRx,
			method,
			weight)
		if err != nil {
			// TODO: review log and fail fast
			log.Errorf("failed to handle route: %v", err)
			return nil, err
		}
		return r, nil
	}
	return nil, fmt.Errorf("backend not found in reference table")
}

// transformRoute creates one eskip.Route for the specified input
func transformRoute(sr *routeSpec, be *skipperBackend, rid, hostRx, method string, weight int) (*eskip.Route, error) {
	ri := &eskip.Route{Id: rid}

	// Path or PathSubtree, prefer Path if we have
	if sr.Path != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "Path", sr.Path)
	} else if sr.PathSubtree != "" {
		ri.Predicates = appendPredicate(ri.Predicates, "PathSubtree", sr.PathSubtree)
	}

	if sr.PathRegexp != "" {
		// TODO: validate correctness
		ri.PathRegexps = []string{"PathRegexp(" + sr.PathRegexp + ")"}
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

	ri.BackendType = be.Type
	ri.Backend = be.String()
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
