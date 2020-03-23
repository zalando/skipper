package eskip

import (
	"fmt"
	"testing"
)

func TestValidatePredicates(t *testing.T) {
	for _, test := range []struct {
		title       string
		predicates  []*Predicate
		expectedErr error
	}{
		{
			title:       "empty predicates",
			predicates:  []*Predicate{},
			expectedErr: nil,
		},
		{
			title: "no conflicts",
			predicates: []*Predicate{
				{Name: "Weight", Args: []interface{}{float64(1)}},
			},
			expectedErr: nil,
		},
		{
			title: "weight conflict",
			predicates: []*Predicate{
				{Name: "Weight", Args: []interface{}{float64(1)}},
				{Name: "Weight", Args: []interface{}{float64(2)}},
			},
			expectedErr: fmt.Errorf("predicate of type %s can only be added once", "Weight"),
		},
		{
			title: "path and pathsubtree conflict",
			predicates: []*Predicate{
				{Name: "Path", Args: []interface{}{"/"}},
				{Name: "PathSubtree", Args: []interface{}{"/"}},
			},
			expectedErr: fmt.Errorf("predicate of type %s cannot be mixed with predicate of type %s", "Path", "PathSubtree"),
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			err := ValidatePredicates(test.predicates)
			if err == nil && test.expectedErr == nil {
				return
			}
			if err != nil && test.expectedErr == nil {
				t.Errorf(`failed validating predicates, expected no error but got "%v"`, err)
				return
			}
			if err == nil && test.expectedErr != nil {
				t.Errorf(`failed validating predicates, expected error "%v" but got none`, test.expectedErr)
				return
			}
			if err.Error() != test.expectedErr.Error() {
				t.Errorf(`failed validating predicates, expected "%v" got "%v"`, test.expectedErr, err)
				return
			}
		})
	}
}
