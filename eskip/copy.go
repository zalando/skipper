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
	if len(p) == 0 {
		return c
	}

	totalArgs := 0
	nonNil := 0
	for _, pi := range p {
		if pi != nil {
			nonNil++
			totalArgs += len(pi.Args)
		}
	}

	var predSlab []Predicate
	if nonNil > 0 {
		predSlab = make([]Predicate, nonNil)
	}
	var argsSlab []interface{}
	if totalArgs > 0 {
		argsSlab = make([]interface{}, totalArgs)
	}

	idx := 0
	for i, pi := range p {
		if pi == nil {
			continue
		}
		predSlab[idx].Name = pi.Name
		if n := len(pi.Args); n > 0 {
			args := argsSlab[:n:n]
			copy(args, pi.Args)
			predSlab[idx].Args = args
			argsSlab = argsSlab[n:]
		} else {
			predSlab[idx].Args = make([]interface{}, 0)
		}
		c[i] = &predSlab[idx]
		idx++
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
	if len(f) == 0 {
		return c
	}

	totalArgs := 0
	nonNil := 0
	for _, fi := range f {
		if fi != nil {
			nonNil++
			totalArgs += len(fi.Args)
		}
	}

	var filterSlab []Filter
	if nonNil > 0 {
		filterSlab = make([]Filter, nonNil)
	}
	var argsSlab []interface{}
	if totalArgs > 0 {
		argsSlab = make([]interface{}, totalArgs)
	}

	idx := 0
	for i, fi := range f {
		if fi == nil {
			continue
		}
		filterSlab[idx].Name = fi.Name
		if n := len(fi.Args); n > 0 {
			args := argsSlab[:n:n]
			copy(args, fi.Args)
			filterSlab[idx].Args = args
			argsSlab = argsSlab[n:]
		} else {
			filterSlab[idx].Args = make([]interface{}, 0)
		}
		c[i] = &filterSlab[idx]
		idx++
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
