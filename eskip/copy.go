package eskip

func copyArgs(a []interface{}) []interface{} {
	// we don't need deep copy of the items for the supported values
	c := make([]interface{}, len(a))
	copy(c, a)
	return c
}

// CopyPredicate creates a copy of the input predicate.
func CopyPredicate(p *Predicate) *Predicate {
	if p == nil {
		return nil
	}

	c := &Predicate{}
	c.Name = p.Name
	c.Args = copyArgs(p.Args)
	return c
}

// CopyPredicates creates a new slice with the copy of each predicate in the input slice.
func CopyPredicates(p []*Predicate) []*Predicate {
	c := make([]*Predicate, len(p))
	for i, pi := range p {
		c[i] = CopyPredicate(pi)
	}

	return c
}

// CopyFilter creates a copy of the input filter.
func CopyFilter(f *Filter) *Filter {
	if f == nil {
		return nil
	}

	c := &Filter{}
	c.Name = f.Name
	c.Args = copyArgs(f.Args)
	return c
}

// CopyFilters creates a new slice with the copy of each filter in the input slice.
func CopyFilters(f []*Filter) []*Filter {
	c := make([]*Filter, len(f))
	for i, fi := range f {
		c[i] = CopyFilter(fi)
	}

	return c
}

// Copy creates a canonical copy of the input route. See also Canonical().
func Copy(r *Route) *Route {
	if r == nil {
		return nil
	}

	r = Canonical(r)
	c := &Route{}
	c.Id = r.Id
	c.Predicates = CopyPredicates(r.Predicates)
	c.Filters = CopyFilters(r.Filters)
	c.BackendType = r.BackendType
	c.Backend = r.Backend
	c.LBAlgorithm = r.LBAlgorithm
	c.LBEndpoints = make([]*LBEndpoint, len(r.LBEndpoints))
	copy(c.LBEndpoints, r.LBEndpoints)
	return c
}

// CopyRoutes creates a new slice with the canonical copy of each route in the input slice.
func CopyRoutes(r []*Route) []*Route {
	c := make([]*Route, len(r))
	for i, ri := range r {
		c[i] = Copy(ri)
	}

	return c
}
