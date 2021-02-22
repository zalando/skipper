package core

import (
	"fmt"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

// ProcessPathOrSubTree processes the Path and PathSubtree predicate arguments.
// returns the subtree path if it is a valid definition
func ProcessPathOrSubTree(p *eskip.Predicate) (string, error) {
	if len(p.Args) != 1 {
		return "", predicates.ErrInvalidPredicateParameters
	}

	if s, ok := p.Args[0].(string); ok {
		return s, nil
	}

	return "", predicates.ErrInvalidPredicateParameters
}

func ValidateHostRegexpPredicate(p *eskip.Predicate) ([]string, error) {
	return getFreeStringArgs(1, p)
}

func ValidatePathRegexpPredicate(p *eskip.Predicate) ([]string, error) {
	return getFreeStringArgs(1, p)
}

func ValidateMethodPredicate(p *eskip.Predicate) ([]string, error) {
	return getFreeStringArgs(1, p)
}

func ValidateHeaderPredicate(p *eskip.Predicate) ([]string, error) {
	return getFreeStringArgs(2, p)
}

func ValidateHeaderRegexpPredicate(p *eskip.Predicate) ([]string, error) {
	return getFreeStringArgs(2, p)
}

func getFreeStringArgs(count int, p *eskip.Predicate) ([]string, error) {
	if len(p.Args) != count {
		return nil, fmt.Errorf(
			"invalid length of predicate args in %s, %d instead of %d",
			p.Name,
			len(p.Args),
			count,
		)
	}

	var a []string
	for i := range p.Args {
		s, ok := p.Args[i].(string)
		if !ok {
			return nil, fmt.Errorf("expected argument of type string, %s", p.Name)
		}

		a = append(a, s)
	}

	return a, nil
}
