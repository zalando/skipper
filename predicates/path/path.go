package path

import (
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
