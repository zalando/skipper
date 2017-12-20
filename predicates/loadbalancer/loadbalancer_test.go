package loadbalancer

import (
	"net/http"
	"reflect"
	"testing"
	"sync"

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
			"fails to create if arguments are not valid",
			[]interface{}{"a", 8, 7},
			nil,
			true,
		},
		{
			"creates a predicate with given group name, index, and count",
			[]interface{}{"a", 1, 7},
			&predicate{
				mu: &sync.RWMutex{},
				group: "a",
				index: 1,
				count: 7,
				counters: map[string]int{
					"a": 0,
				},
			},
			false,
		},
		{
			"creates a predicate with given group name, index, and count with floats",
			[]interface{}{"a", 1.0, 7.0},
			&predicate{
				mu: &sync.RWMutex{},
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
			s := &spec{mu: &sync.RWMutex{}}
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

func TestMatch(t *testing.T) {
	tests := []struct {
		name           string
		predicateArgs  [][]interface{}
		numRequests    int
		matchedIndices []int
	}{
		{
			"should match the predicates in round-robin fashion",
			[][]interface{}{
				[]interface{}{
					"a",
					0,
					3,
				},
				[]interface{}{
					"a",
					1,
					3,
				},
				[]interface{}{
					"a",
					2,
					3,
				},
			},
			5,
			[]int{0, 1, 2, 0, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			var preds []routing.Predicate
			for _, args := range tt.predicateArgs {
				p, err := s.Create(args)
				if err != nil {
					t.Fatal(err)
				}
				preds = append(preds, p)
			}
			var matchedIndices []int
			for i := 0; i < tt.numRequests; i++ {
				req := &http.Request{}
				for _, p := range preds {
					pred, ok := p.(*predicate)
					if !ok {
						t.Fatal("expected a loadbalancer predicate")
					}
					if pred.Match(req) {
						matchedIndices = append(matchedIndices, pred.index)
						break
					}
				}
			}
			if !reflect.DeepEqual(matchedIndices, tt.matchedIndices) {
				t.Errorf("expected predicate with index %v to be matched, instead %v matched", tt.matchedIndices, matchedIndices)
			}
		})
	}
}
