package kubernetes

import (
	"fmt"
	"strings"

	"github.com/zalando/skipper/eskip"
)

func eastWestRouteID(rid string) string {
	return "kubeew" + rid[len(ingressRouteIDPrefix):]
}

func createEastWestRouteIng(eastWestDomainRegexpPostfix, name, ns string, r *eskip.Route) *eskip.Route {
	if strings.HasPrefix(r.Id, "kubeew") || ns == "" || name == "" {
		return nil
	}
	ewR := *r
	ewR.HostRegexps = []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ewR.Id = eastWestRouteID(r.Id)
	return &ewR
}

func createEastWestRoutesIng(eastWestDomainRegexpPostfix, name, ns string, routes []*eskip.Route) []*eskip.Route {
	ewroutes := make([]*eskip.Route, 0)
	newHostRegexps := []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ingressAlreadyHandled := false

	for _, r := range routes {
		// TODO(sszuecs) we have to rethink how to handle eastwest routes in more complex cases
		n := countPathRoutes(r)
		// FIX memory leak in route creation
		if strings.HasPrefix(r.Id, "kubeew") || (n == 0 && ingressAlreadyHandled) {
			continue
		}
		r.Namespace = ns // store namespace
		r.Name = name    // store name
		ewR := *r
		ewR.HostRegexps = newHostRegexps
		ewR.Id = eastWestRouteID(r.Id)
		ewroutes = append(ewroutes, &ewR)
		ingressAlreadyHandled = true
	}
	return ewroutes
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
		Args: []interface{}{hostRx},
	})

	ewr.Predicates = p
	return ewr
}
