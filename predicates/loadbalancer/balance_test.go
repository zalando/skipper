package loadbalancer

import (
	"reflect"
	"testing"

	"github.com/zalando/skipper/eskip"
)

func checkUnsorted(left, right []*eskip.Route) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		var found bool
		for j := range right {
			if reflect.DeepEqual(left[i], right[j]) {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func TestBalanceRoute(t *testing.T) {
	for _, test := range []struct {
		title          string
		route          *eskip.Route
		backends       []string
		expectedRoutes []*eskip.Route
	}{{
		title: "no endpoint",
		route: &eskip.Route{
			Id:      "foo",
			Backend: "https://foo",
		},
	}, {
		title: "one backend",
		route: &eskip.Route{
			Id:   "foo",
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo",
		},
		backends: []string{"https://foo1"},
		expectedRoutes: []*eskip.Route{{
			Id:   "foo",
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: groupPredicateName,
				Args: []interface{}{createGroupName("foo")},
			}},
			Filters: []*eskip.Filter{{
				Name: decideFilterName,
				Args: []interface{}{createGroupName("foo"), 1},
			}},
			BackendType: eskip.LoopBackend,
		}, {
			Id:   createMemberID("foo", 0),
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: memberPredicateName,
				Args: []interface{}{createGroupName("foo"), 0},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo1",
		}},
	}, {
		title: "three backends",
		route: &eskip.Route{
			Id:   "foo",
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo",
		},
		backends: []string{"https://foo1", "https://foo2", "https://foo3"},
		expectedRoutes: []*eskip.Route{{
			Id:   "foo",
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: groupPredicateName,
				Args: []interface{}{createGroupName("foo")},
			}},
			Filters: []*eskip.Filter{{
				Name: decideFilterName,
				Args: []interface{}{createGroupName("foo"), 3},
			}},
			BackendType: eskip.LoopBackend,
		}, {
			Id:   createMemberID("foo", 0),
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: memberPredicateName,
				Args: []interface{}{createGroupName("foo"), 0},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo1",
		}, {
			Id:   createMemberID("foo", 1),
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: memberPredicateName,
				Args: []interface{}{createGroupName("foo"), 1},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo2",
		}, {
			Id:   createMemberID("foo", 2),
			Path: "/path",
			Predicates: []*eskip.Predicate{{
				Name: "Pred1",
				Args: []interface{}{1, 2, 3},
			}, {
				Name: "Pred2",
				Args: []interface{}{4, 5, 6},
			}, {
				Name: memberPredicateName,
				Args: []interface{}{createGroupName("foo"), 2},
			}},
			Filters: []*eskip.Filter{{
				Name: "filter1",
				Args: []interface{}{7, 8, 9},
			}, {
				Name: "filter2",
				Args: []interface{}{7, 8, 9},
			}},
			Backend: "https://foo3",
		}},
	}} {
		t.Run(test.title, func(t *testing.T) {
			balancedRoutes := BalanceRoute(test.route, test.backends)
			if !checkUnsorted(balancedRoutes, test.expectedRoutes) {
				t.Error("failed to balance routes")
				t.Log("got:     ", balancedRoutes)
				t.Log("expected:", test.expectedRoutes)
			}
		})
	}
}
