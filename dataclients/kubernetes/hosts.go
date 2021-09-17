package kubernetes

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/zalando/skipper/eskip"
)

func createHostRx(hosts ...string) string {
	if len(hosts) == 0 {
		return ""
	}

	hrx := make([]string, len(hosts))
	for i, host := range hosts {
		hrx[i] = strings.Replace(host, ".", "[.]", -1) + "(:[0-9]+)?"
	}

	return "^(" + strings.Join(hrx, "|") + ")$"
}

// hostCatchAllRoutes creates catch-all routes for those hosts that only have routes with
// a Host predicate and at least one additional predicate.
//
// currently only used for RouteGroups
func hostCatchAllRoutes(hostRoutes map[string][]*eskip.Route, createID func(string) string) []*eskip.Route {
	var catchAll []*eskip.Route
	for h, r := range hostRoutes {
		var hasHostOnlyRoute bool
		for _, ri := range r {
			ct := eskip.Canonical(ri)
			var hasNonHostPredicate bool
			for _, p := range ct.Predicates {
				if p.Name != "Host" {
					hasNonHostPredicate = true
					break
				}
			}

			if !hasNonHostPredicate {
				hasHostOnlyRoute = true
				break
			}
		}

		if !hasHostOnlyRoute {
			catchAll = append(catchAll, &eskip.Route{
				Id: createID(h),
				Predicates: []*eskip.Predicate{{
					Name: "Host",
					Args: []interface{}{createHostRx(h)},
				}},
				BackendType: eskip.ShuntBackend,
			})
		}
	}

	return catchAll
}

func isExternalDomainAllowed(allowedDomains []*regexp.Regexp, domain string) bool {
	for _, a := range allowedDomains {
		if a.MatchString(domain) {
			return true
		}
	}

	return false
}

func isExternalAddressAllowed(allowedDomains []*regexp.Regexp, address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}

	return isExternalDomainAllowed(allowedDomains, u.Hostname())
}
