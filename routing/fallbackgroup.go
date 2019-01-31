package routing

// QUESTION: does the fallback functionality always apply to load balanced routes?
// TODO: implement an additional predicate that is also checked for fallbacks, sg. like "fallback group"

// currently only for load balanced routes
func applyFallbackGroups(r []*Route) []*Route {
	for i := range r {
		l := len(r[i].Filters)
		if l < 1 {
			continue
		}

		f := r[i].Filters[l-1]
		switch f.Name {
		case "roundRobin":
			r[i].IsLoadBalanced = true
		}
	}

	return r
}
