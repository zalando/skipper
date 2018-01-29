package routing

// QUESTION: does the fallback functionality always apply to load balanced routes?
// TODO: implement an additional predicate that is also checked for fallbacks, sg. like "fallback group"

// currently only for load balanced routes
func applyFallbackGroups(r []*Route) []*Route {
	groups := make(map[string][]*Route)
	for i := range r {
		for j := range r[i].Predicates {
			er := r[i].Route

			// NOTE: this one "LBMember" is now hard coded. It can be fixed if we implement this
			// logic as a post processor.
			if er.Predicates[j].Name != "LBMember" {
				continue
			}

			if len(er.Predicates[j].Args) == 0 {
				continue
			}

			name, ok := er.Predicates[j].Args[0].(string)
			if !ok {
				continue
			}

			groups[name] = append(groups[name], r[i])
		}
	}

	for name, group := range groups {
		if len(group) == 0 {
			continue
		}

		// TODO: here we could clean off load balancing, if there's only a single route

		head := group[0]
		head.Head = head
		head.Me = head
		head.Group = name
		head.IsLoadBalanced = true

		current := head
		for _, route := range group[1:] {
			current.Next = route
			current = route
			current.Head = head
			current.Me = current
			current.Group = name
			current.IsLoadBalanced = true
		}
	}

	return r
}
