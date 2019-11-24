package eskip

func copyArgs(a []interface{}) []interface{} {
	// we don't need deep copy of the items for the supported values
	c := make([]interface{}, len(a))
	copy(c, a)
	return c
}

func CopyPredicate(p *Predicate) *Predicate {
	if p == nil {
		return nil
	}

	c := &Predicate{}
	c.Name = p.Name
	c.Args = copyArgs(p.Args)
	return c
}

func CopyPredicates(p []*Predicate) []*Predicate {
	c := make([]*Predicate, len(p))
	for i, pi := range p {
		c[i] = CopyPredicate(pi)
	}

	return c
}

func CopyFilter(f *Filter) *Filter {
	if f == nil {
		return nil
	}

	c := &Filter{}
	c.Name = f.Name
	c.Args = copyArgs(f.Args)
	return c
}

func CopyFilters(f []*Filter) []*Filter {
	c := make([]*Filter, len(f))
	for i, fi := range f {
		c[i] = CopyFilter(fi)
	}

	return c
}

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
	c.LBEndpoints = make([]string, len(r.LBEndpoints))
	copy(c.LBEndpoints, r.LBEndpoints)
	return c
}

func CopyRoutes(r []*Route) []*Route {
	c := make([]*Route, len(r))
	for i, ri := range r {
		c[i] = Copy(ri)
	}

	return c
}
