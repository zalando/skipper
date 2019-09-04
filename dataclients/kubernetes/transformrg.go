package kubernetes

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"strings"

	"github.com/zalando/skipper/eskip"
)

func createHostRx(h []string) string {
	return fmt.Sprintf("^(%s)$", strings.Join(h, "|"))
}

// expects at least one
func mapBackends(b []*skipperBackend) (m map[string]*skipperBackend, dflt *skipperBackend) {
	m = make(map[string]*skipperBackend)
	for _, bi := range b {
		m[bi.Name] = bi
		if bi.Default {
			dflt = bi
		}
	}

	if dflt == nil {
		dflt = b[0]
	}

	return
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

	if len(rg.Spec.Paths) == 0 {
		return nil, fmt.Errorf("missing path spec for route group: %s", rg.Metadata.Name)
	}

	hostRx := createHostRx(rg.Spec.Hosts)
	bm, dflt := mapBackends(rg.Spec.Backends)

	var r []*eskip.Route
	for i, p := range rg.Spec.Paths {
		ri := &eskip.Route{Id: crdRouteID(rg.Metadata, p.Path, i)}
		if p.Path != "" {
			ri.Predicates = appendPredicate(ri.Predicates, "Path", p.Path)
		}

		if len(rg.Spec.Hosts) > 0 {
			ri.Predicates = appendPredicate(ri.Predicates, "Host", hostRx)
		}

		if p.Method != "" {
			ri.Predicates = appendPredicate(ri.Predicates, "Method", strings.ToUpper(p.Method))
		}

		var pp []*eskip.Predicate
		for _, pi := range p.Predicates {
			ppi, err := eskip.ParsePredicates(pi)
			if err != nil {
				return nil, err
			}

			pp = append(pp, ppi...)
		}

		ri.Predicates = append(ri.Predicates, pp...)

		var f []*eskip.Filter
		for _, fi := range p.Filters {
			ffi, err := eskip.ParseFilters(fi)
			if err != nil {
				return nil, err
			}

			f = append(f, ffi...)
		}

		ri.Filters = f

		// TODO: properly
		b := dflt
		if p.Backend != "" {
			bref, ok := bm[p.Backend]
			if !ok {
				return nil, fmt.Errorf(
					"missing backend in route group: %s, for path: %s, with reference: %s",
					rg.Metadata.Name,
					p.Path,
					p.Backend,
				)
			}

			b = bref
		}

		ri.Backend = fmt.Sprintf("http://%s", b.ServiceName)
		r = append(r, ri)
	}

	return r, nil
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
