package kubernetes

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/zalando/skipper/eskip"
)

func rxDots(h string) string {
	return strings.Replace(h, ".", "[.]", -1)
}

func createHostRx(h ...string) string {
	if len(h) == 0 {
		return ""
	}

	hrx := make([]string, len(h))
	for i := range h {
		hrx[i] = rxDots(h[i])
	}

	return fmt.Sprintf("^(%s)$", strings.Join(hrx, "|"))
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
