package definitions

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
)

func FromEskip(r []*eskip.Route) (*RouteGroupItem, error) {
	r = eskip.CopyRoutes(r)
	unique := make(map[string]*eskip.Route)
	for _, ri := range r {
		unique[ri.Id] = ri
	}

	r = nil
	for _, ri := range unique {
		r = append(r, ri)
	}

	var rg RouteGroupItem
	rg.Metadata = &Metadata{}
	rg.Spec = &RouteGroupSpec{}
	backendNames := make(map[string]string)
	backendsByName := make(map[string]*SkipperBackend)
	backendReferences := make(map[string]int)
	backends := make(map[*eskip.Route]*SkipperBackend)
	backendName := func(key string) string {
		name, ok := backendNames[key]
		if !ok {
			name = fmt.Sprintf("backend%d", len(backendNames))
			backendNames[key] = name
		}

		return name
	}

	simpleBackend := func(typ eskip.BackendType, key string) *SkipperBackend {
		name := backendName(key)
		backendReferences[name]++
		b, ok := backendsByName[name]
		if ok {
			return b
		}

		b = &SkipperBackend{
			Name: name,
			Type: typ,
		}

		backendsByName[name] = b
		return b
	}

	var err error
	for _, ri := range r {
		switch ri.BackendType {
		case eskip.ShuntBackend:
			backends[ri] = simpleBackend(eskip.ShuntBackend, "<shunt>")
		case eskip.LoopBackend:
			backends[ri] = simpleBackend(eskip.LoopBackend, "<loopback>")
		case eskip.DynamicBackend:
			backends[ri] = simpleBackend(eskip.DynamicBackend, "<dynamic>")
		case eskip.LBBackend:
			key := strings.Join(append([]string{ri.LBAlgorithm}, ri.LBEndpoints...), ",")
			name := backendName(key)
			backendReferences[name]++
			b, ok := backendsByName[name]
			if ok {
				backends[ri] = b
				continue
			}

			b = &SkipperBackend{
				Type: eskip.LBBackend,
				Name: name,
			}

			if b.Algorithm, err = loadbalancer.AlgorithmFromString(ri.LBAlgorithm); err != nil {
				return nil, err
			}

			if b.Algorithm == loadbalancer.None {
				b.Algorithm = loadbalancer.RoundRobin
			}

			b.Endpoints = ri.LBEndpoints
			backendsByName[name] = b
			backends[ri] = b
		default:
			name := backendName(ri.Backend)
			backendReferences[name]++
			b, ok := backendsByName[name]
			if ok {
				backends[ri] = b
				continue
			}

			b = &SkipperBackend{
				Name: name,
			}

			var u *url.URL
			if u, err = url.Parse(ri.Backend); err != nil {
				return nil, err
			}

			if u.Scheme == "service" {
				b.Type = ServiceBackend
				b.ServiceName = u.Hostname()
				if b.ServicePort, err = strconv.Atoi(u.Port()); err != nil {
					return nil, err
				}

				b.Algorithm = loadbalancer.RoundRobin
			} else {
				b.Type = eskip.NetworkBackend
				b.Address = ri.Backend
			}

			backendsByName[name] = b
			backends[ri] = b
		}
	}

	for _, b := range backendsByName {
		rg.Spec.Backends = append(rg.Spec.Backends, b)
	}

	var (
		defaultBackend string
		maxRefs        int
	)

	for name, refs := range backendReferences {
		if refs > 1 && refs > maxRefs {
			maxRefs = refs
			defaultBackend = name
		}
	}

	if maxRefs > 1 {
		rg.Spec.DefaultBackends = []*BackendReference{{
			BackendName: defaultBackend,
		}}
	}

	for _, ri := range r {
		rs := &RouteSpec{}
		if backends[ri].Name != defaultBackend {
			rs.Backends = []*BackendReference{{BackendName: backends[ri].Name}}
		}

		for _, p := range ri.Predicates {
			var ok bool
			switch p.Name {
			case "Path":
				if len(p.Args) == 1 {
					rs.Path, ok = p.Args[0].(string)
				}
			case "PathSubtree":
				if len(p.Args) == 1 {
					rs.PathSubtree, ok = p.Args[0].(string)
				}
			case "PathRegexp":
				if len(p.Args) == 1 {
					rs.PathRegexp, ok = p.Args[0].(string)
				}
			case "Method":
				if len(p.Args) == 1 {
					var m string
					m, ok = p.Args[0].(string)
					if ok {
						rs.Methods = []string{m}
					}
				}
			case "Methods":
				var m string
				ok = true
				for _, a := range p.Args {
					m, ok = a.(string)
					if !ok {
						break
					}

					rs.Methods = append(rs.Methods, m)
				}
			default:
				rs.Predicates = append(rs.Predicates, p.String())
				ok = true
			}

			if !ok {
				return nil, fmt.Errorf(
					"invalid predicate type in route: %s, predicate: %s",
					ri.Id,
					p.Name,
				)
			}
		}

		for _, f := range ri.Filters {
			rs.Filters = append(rs.Filters, f.String())
		}

		rg.Spec.Routes = append(rg.Spec.Routes, rs)
	}

	return &rg, nil
}
