package weight

import "errors"

// ErrInvalidWeightParams is used in case of invalid weight predicate parameters.
var ErrInvalidWeightParams = errors.New("invalid argument for the Weight predicate")

// ParseWeightPredicateArgs parses the weight predicate arguments.
func ParseWeightPredicateArgs(args []interface{}) (int, error) {
	if len(args) != 1 {
		return 0, ErrInvalidWeightParams
	}

	if weight, ok := args[0].(float64); ok {
		return int(weight), nil
	}

	if weight, ok := args[0].(int); ok {
		return weight, nil
	}

	return 0, ErrInvalidWeightParams
}
