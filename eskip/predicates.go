package eskip

import (
	"fmt"
)

// PredicatesContain checks if the route has predicate with the given name.
func PredicatesContain(p []*Predicate, name string) bool {
	for _, p := range p {
		if p.Name == name {
			return true
		}
	}

	return false
}

// AllPredicatesByName returns all predicates that match the given name.
func AllPredicatesByName(p []*Predicate, name string) []*Predicate {
	pp := make([]*Predicate, 0)
	for _, p := range p {
		if p.Name == name {
			pp = append(pp, p)
		}
	}

	return pp
}

// SinglePredicateByName returns matching predicate or error when multiple predicates are given.
func SinglePredicateByName(p []*Predicate, name string) (*Predicate, error) {
	pp := make([]*Predicate, 0)
	for _, p := range p {
		if p.Name == name {
			pp = append(pp, p)
		}
	}

	if len(pp) == 0 {
		return nil, nil
	}

	if len(pp) > 1 {
		return nil, fmt.Errorf("multiple predicates of the same name: %d %s", len(pp), name)
	}

	return pp[0], nil
}

// ValidatePredicates returns an error when certain predicates are added multiple times.
func ValidatePredicates(pp []*Predicate) error {
	counts := make(map[string]int)
	for _, p := range pp {
		counts[p.Name]++
	}

	if counts["Weight"] > 1 {
		return fmt.Errorf("predicate of type %s can only be added once", "Weight")
	}

	if counts["Path"] > 0 && counts["PathSubtree"] > 0 {
		return fmt.Errorf("predicate of type %s cannot be mixed with predicate of type %s", "Path", "PathSubtree")
	}

	return nil
}

// PrependPredicate prepends all predicates to the existing predicates and validates them.
func (r *Route) PrependPredicate(pp ...*Predicate) error {
	next := append(pp, r.Predicates...)
	err := ValidatePredicates(next)
	if err != nil {
		return err
	}
	r.Predicates = next

	return nil
}

// AppendPredicate appends all predicates to the existing predicates and validates them.
func (r *Route) AppendPredicate(pp ...*Predicate) error {
	next := append(r.Predicates, pp...)
	err := ValidatePredicates(next)
	if err != nil {
		return err
	}
	r.Predicates = next

	return nil
}

// ReplacePredicates replaces all predicates and validates them.
func (r *Route) ReplacePredicates(pp ...*Predicate) error {
	next := pp
	err := ValidatePredicates(next)
	if err != nil {
		return err
	}
	r.Predicates = next

	return nil
}
