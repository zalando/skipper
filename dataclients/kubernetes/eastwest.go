package kubernetes

import (
	"fmt"
	"strings"

	"github.com/zalando/skipper/eskip"
)

func eastWestRouteID(rid string) string {
	return "kubeew" + rid[len(ingressRouteIDPrefix):]
}

func createEastWestRouteIng(eastWestDomain, name, ns string, r *eskip.Route) *eskip.Route {
	if strings.HasPrefix(r.Id, "kubeew") || ns == "" || name == "" {
		return nil
	}
	ewR := *r
	ewR.HostRegexps = []string{createHostRx(name + "." + ns + "." + eastWestDomain)}
	ewR.Id = eastWestRouteID(r.Id)
	return &ewR
}

func createEastWestRouteRG(name, ns, postfix string, r *eskip.Route) *eskip.Route {
	hostRx := createHostRx(fmt.Sprintf("%s.%s.%s", name, ns, postfix))

	ewr := eskip.Copy(r)
	ewr.Id = eastWestRouteID(ewr.Id)
	ewr.HostRegexps = nil

	p := make([]*eskip.Predicate, 0, len(ewr.Predicates))
	for _, pi := range ewr.Predicates {
		if pi.Name != "Host" {
			p = append(p, pi)
		}
	}

	p = append(p, &eskip.Predicate{
		Name: "Host",
		Args: []any{hostRx},
	})

	ewr.Predicates = p
	return ewr
}

func applyEastWestRange(domains []string, predicates []*eskip.Predicate, host string, routes []*eskip.Route) {
	for _, d := range domains {
		if strings.HasSuffix(host, d) {
			applyEastWestRangePredicates(routes, predicates)
		}
	}
}

func applyEastWestRangePredicates(routes []*eskip.Route, predicates []*eskip.Predicate) {
	for _, route := range routes {
		route.Predicates = append(route.Predicates, predicates...)
	}
}
