package roundrobin

import (
	"reflect"
	"testing"

	"github.com/zalando/skipper/routing"
)

func TestSpecCreate(t *testing.T) {
	tests := []struct {
		name    string
		args    []interface{}
		want    routing.Predicate
		wantErr bool
	}{
		{
			"fails to create if there are less than 3 arguments",
			nil,
			nil,
			true,
		},
		{
			"fails to create if there are less than 3 arguments",
			[]interface{}{"a"},
			nil,
			true,
		},
		{
			"fails to create if there are less than 3 arguments",
			[]interface{}{"a", "b"},
			nil,
			true,
		},
		{
			"fails to create if arguments are not valid",
			[]interface{}{"a", "b", "c"},
			nil,
			true,
		},
		{
			"fails to create if arguments are not valid",
			[]interface{}{"a", 1, "c"},
			nil,
			true,
		},
		{
			"fails to create if arguments are not valid",
			[]interface{}{"a", -1, 10},
			nil,
			true,
		},
		{
			"fails to create if arguments are not valid",
			[]interface{}{"", 0, 1},
			nil,
			true,
		},
		{
			"fails to create if arguments are not valid",
			[]interface{}{"a", 0, 0},
			nil,
			true,
		},
		{
			"creates a predicate with given group name, index, and count",
			[]interface{}{"a", 1, 7},
			&predicate{
				group: "a",
				index: 1,
				count: 7,
				counters: map[string]int{
					"a": 0,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &spec{}
			got, err := s.Create(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("spec.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("spec.Create() = %v, want %v", got, tt.want)
			}
		})
	}
}
