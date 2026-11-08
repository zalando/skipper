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
		// trailing dots and port are not allowed in kube
		// ingress spec, so we can append optional setting
		// without check
		hrx[i] = strings.ReplaceAll(host, ".", "[.]") + "[.]?(:[0-9]+)?"
	}

	return "^(" + strings.Join(hrx, "|") + ")$"
}

func createHostPredicate(hosts ...string) *eskip.Predicate {
	if len(hosts) == 0 {
		return nil
	}
	args := make([]any, 0, 2*len(hosts))
	for _, host := range hosts {
		if host == "" || strings.IndexFunc(host, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
				c >= '0' && c <= '9' || c == '-' || c == '.')
		}) >= 0 {
			return &eskip.Predicate{Name: "Host", Args: []any{createHostRx(hosts...)}}
		}
		// Normalization removes one trailing dot. Keep the old match for
		// requests that still have one after normalization.
		args = append(args, host, host+".")
	}
	return &eskip.Predicate{Name: "HostAny", Args: args}
}

func setHostPredicate(r *eskip.Route, host string) {
	if r == nil || len(r.HostRegexps) == 0 {
		return
	}
	r.HostRegexps = nil
	r.Predicates = append([]*eskip.Predicate{createHostPredicate(host)}, r.Predicates...)
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
			route := *ri
			// The first predicate is the generated group host condition.
			// User-supplied HostAny predicates remain additional conditions.
			if len(route.Predicates) > 0 && route.Predicates[0].Name == "HostAny" {
				route.Predicates = route.Predicates[1:]
			}
			ct := eskip.Canonical(&route)
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
				Id:          createID(h),
				Predicates:  []*eskip.Predicate{createHostPredicate(h)},
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

func isExternalRouteAllowed(allowedDomains []*regexp.Regexp, r *eskip.Route) bool {
	switch r.BackendType {
	case eskip.NetworkBackend:
		return isExternalAddressAllowed(allowedDomains, r.Backend)
	case eskip.LBBackend:
		for _, ep := range r.LBEndpoints {
			if !isExternalAddressAllowed(allowedDomains, ep.Address) {
				return false
			}
		}
	}
	return true
}

func isEastWestHost(host string, eastWestRangeDomains []string) bool {
	for _, domainSuffix := range eastWestRangeDomains {
		if strings.HasSuffix(host, domainSuffix) {
			return true
		}
	}
	return false
}
