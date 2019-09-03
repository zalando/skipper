package eskip

import "sort"

// used for sorting:
func compareRouteID(r []*Route) func(int, int) bool {
	return func(i, j int) bool {
		return r[i].Id < r[j].Id
	}
}

// used for sorting:
func comparePredicateName(p []*Predicate) func(int, int) bool {
	return func(i, j int) bool {
		return p[i].Name < p[j].Name
	}
}

func hasDuplicateID(r []*Route) bool {
	for i := 1; i < len(r); i++ {
		if r[i-1].Id == r[i].Id {
			return true
		}
	}

	return false
}

// fixing legacy fields, and sorting predicates by name:
func canonical(r *Route) *Route {
	if r == nil {
		return nil
	}

	c := &Route{}
	c.Id = r.Id

	c.Predicates = make([]*Predicate, len(r.Predicates))
	copy(c.Predicates, r.Predicates)

	// legacy path:
	var hasPath bool
	for _, p := range c.Predicates {
		if p.Name == "Path" {
			hasPath = true
			break
		}
	}

	if r.Path != "" && !hasPath {
		c.Predicates = append(c.Predicates, &Predicate{Name: "Path", Args: []interface{}{r.Path}})
	}

	// legacy host:
	for _, h := range r.HostRegexps {
		c.Predicates = append(c.Predicates, &Predicate{Name: "Host", Args: []interface{}{h}})
	}

	// legacy path regexp:
	for _, p := range r.PathRegexps {
		c.Predicates = append(c.Predicates, &Predicate{Name: "PathRegexp", Args: []interface{}{p}})
	}

	// legacy method:
	if r.Method != "" {
		// prepend the method, so that the canonical []Predicates will be prioritized in case of
		// duplicates, and imitate how the routing evaluates multiple method predicates, even if
		// weird
		c.Predicates = append(
			[]*Predicate{{Name: "Method", Args: []interface{}{r.Method}}},
			c.Predicates...,
		)
	}

	// legacy header:
	for name, value := range r.Headers {
		c.Predicates = append(
			c.Predicates,
			&Predicate{Name: "Header", Args: []interface{}{name, value}},
		)
	}

	// legacy header regexp:
	for name, values := range r.HeaderRegexps {
		for _, value := range values {
			c.Predicates = append(
				c.Predicates,
				&Predicate{Name: "HeaderRegexp", Args: []interface{}{name, value}},
			)
		}
	}

	sort.Slice(c.Predicates, comparePredicateName(c.Predicates))
	c.Filters = r.Filters

	c.BackendType = r.BackendType
	switch c.BackendType {
	case NetworkBackend:
		// default overridden by legacy shunt:
		if r.Shunt {
			c.BackendType = ShuntBackend
		} else {
			c.Backend = r.Backend
		}
	case LBBackend:
		// using the LB fields only when apply:
		c.LBAlgorithm = r.LBAlgorithm
		c.LBEndpoints = make([]string, len(r.LBEndpoints))
		copy(c.LBEndpoints, r.LBEndpoints)
		sort.Strings(c.LBEndpoints)
	}

	// Name and Namespace stripped

	return c
}

func eqArgs(left, right []interface{}) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}

func eqStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}

func eq2(left, right *Route) bool {
	lc, rc := canonical(left), canonical(right)

	if left == nil && right == nil {
		return true
	}

	if left == nil || right == nil {
		return false
	}

	if len(lc.Predicates) != len(rc.Predicates) {
		return false
	}

	for i := range lc.Predicates {
		lp, rp := lc.Predicates[i], rc.Predicates[i]
		if lp.Name != rp.Name || !eqArgs(lp.Args, rp.Args) {
			return false
		}
	}

	if len(lc.Filters) != len(rc.Filters) {
		return false
	}

	for i := range lc.Filters {
		lf, rf := lc.Filters[i], rc.Filters[i]
		if lf.Name != rf.Name || !eqArgs(lf.Args, rf.Args) {
			return false
		}
	}

	if lc.BackendType != rc.BackendType {
		return false
	}

	if lc.Backend != rc.Backend {
		return false
	}

	if lc.LBAlgorithm != rc.LBAlgorithm {
		return false
	}

	if !eqStrings(lc.LBEndpoints, rc.LBEndpoints) {
		return false
	}

	return true
}

func eq2Lists(left, right []*Route) bool {
	if len(left) != len(right) {
		return false
	}

	sort.Slice(left, compareRouteID(left))
	sort.Slice(right, compareRouteID(right))
	if hasDuplicateID(left) || hasDuplicateID(right) {
		return false
	}

	for i := range left {
		if !eq2(left[i], right[i]) {
			return false
		}
	}

	return true
}

func Eq(r ...*Route) bool {
	for i := 1; i < len(r); i++ {
		if !eq2(r[i-1], r[i]) {
			return false
		}
	}

	return true
}

// Eq implements canonical equivalence comparison of routes based on
// Skipper semantics.
//
// Duplicate IDs are considered invalid for Eq, and it returns false
// in this case.
//
// The Name and Namespace fields are ignored.
//
// If there are multiple methods, only the last one is considered, to
// reproduce the route matching (even if how it works, may not be the
// most expected in regard of the method predicates).
//
// The order of the routes in the lists doesn't matter.
func EqLists(r ...[]*Route) bool {
	for i := 1; i < len(r); i++ {
		if !eq2Lists(r[i-1], r[i]) {
			return false
		}
	}

	return true
}
