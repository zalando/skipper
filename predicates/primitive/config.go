package primitive

import (
	"regexp"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type (
	configSpec struct {
		values map[string]string
	}
)

// NewConfig provides a predicate spec to create predicates
// that evaluate to true if config value matches regular expression
func NewConfig(values map[string]string) routing.PredicateSpec {
	return &configSpec{values}
}

func (*configSpec) Name() string { return predicates.ConfigName }

func (s *configSpec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	name, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	value, ok := args[1].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	re, err := regexp.Compile(value)
	if err != nil {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if re.MatchString(s.values[name]) {
		return &truePredicate{}, nil
	} else {
		return &falsePredicate{}, nil
	}
}
