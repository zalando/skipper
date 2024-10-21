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

func eqLBEndpoints(left, right []*LBEndpoint) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i].Address != right[i].Address || left[i].Zone != right[i].Zone {
			return false
		}
	}

	return true
}

func eq2(left, right *Route) bool {
	lc, rc := Canonical(left), Canonical(right)

	if left == nil && right == nil {
		return true
	}

	if left == nil || right == nil {
		return false
	}

	if lc.Id != rc.Id {
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

	if !eqLBEndpoints(lc.LBEndpoints, rc.LBEndpoints) {
		return false
	}

	return true
}

func eq2Lists(left, right []*Route) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if !eq2(left[i], right[i]) {
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
func Eq(r ...*Route) bool {
	for i := 1; i < len(r); i++ {
		if !eq2(r[i-1], r[i]) {
			return false
		}
	}

	return true
}

// EqLists compares lists of routes. It returns true if the routes contained
// by each list are equal by Eq(). Repeated route IDs are considered invalid
// and EqLists always returns false in this case. The order of the routes in
// the lists doesn't matter.
func EqLists(r ...[]*Route) bool {
	rc := make([][]*Route, len(r))
	for i := range rc {
		rc[i] = make([]*Route, len(r[i]))
		copy(rc[i], r[i])
		sort.Slice(rc[i], compareRouteID(rc[i]))
		if hasDuplicateID(rc[i]) {
			return false
		}
	}

	for i := 1; i < len(rc); i++ {
		if !eq2Lists(rc[i-1], rc[i]) {
			return false
		}
	}

	return true
}

// Canonical returns the canonical representation of a route, that uses the
// standard, non-legacy representation of the predicates and the backends.
// Canonical creates a copy of the route, but doesn't necessarily creates a
// copy of every field. See also Copy().
func Canonical(r *Route) *Route {
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

	if len(c.Predicates) == 0 {
		c.Predicates = nil
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
		c.LBEndpoints = make([]*LBEndpoint, len(r.LBEndpoints))
		copy(c.LBEndpoints, r.LBEndpoints)
		sort.Slice(c.LBEndpoints, func(i, j int) bool {
			return c.LBEndpoints[i].Address < c.LBEndpoints[j].Address
		})
	}

	// Name and Namespace stripped

	return c
}

// CanonicalList returns the canonical form of each route in the list,
// keeping the order. The returned slice is a new slice of the input
// slice but the routes in the slice and their fields are not necessarily
// all copied. See more at CopyRoutes() and Canonical().
func CanonicalList(l []*Route) []*Route {
	if len(l) == 0 {
		return nil
	}

	cl := make([]*Route, len(l))
	for i := range l {
		cl[i] = Canonical(l[i])
	}

	return cl
}
