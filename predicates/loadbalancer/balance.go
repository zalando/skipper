package loadbalancer

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
)

func createGroupName(routeID string) string {
	return fmt.Sprintf("__lb_group_%s", routeID)
}

func createMemberID(routeID string, index int) string {
	return fmt.Sprintf("__lb_route_%s_%d", routeID, index)
}

func createDecisionRoute(original *eskip.Route, groupName string, groupSize int) *eskip.Route {
	dr := *original

	// we keep the original ID, as this is the entry point for this set of routes

	// we keep the original predicates, too, to avoid conflicts with other routing:
	dr.Predicates = append(dr.Predicates, &eskip.Predicate{
		Name: groupPredicateName,
		Args: []interface{}{groupName},
	})

	// original filters only in the member routes:
	dr.Filters = []*eskip.Filter{{
		Name: decideFilterName,
		Args: []interface{}{groupName, groupSize},
	}}

	dr.Shunt = false
	dr.Backend = ""
	dr.BackendType = eskip.LoopBackend

	return &dr
}

func createMember(original *eskip.Route, groupName string, index int, backend string) *eskip.Route {
	m := *original
	m.Id = createMemberID(original.Id, index)

	// we keep the original predicates, too, to avoid conflicts with other routing:
	m.Predicates = append(m.Predicates, &eskip.Predicate{
		Name: memberPredicateName,
		Args: []interface{}{groupName, index},
	})

	m.Shunt = false
	m.BackendType = eskip.NetworkBackend
	m.Backend = backend

	// we keep the original filters to let them do their job

	return &m
}

func createMembers(original *eskip.Route, groupName string, backends []string) []*eskip.Route {
	var members []*eskip.Route
	for i := range backends {
		members = append(members, createMember(original, groupName, i, backends[i]))
	}

	return members
}

func BalanceRoute(r *eskip.Route, backends []string) []*eskip.Route {
	if len(backends) == 0 {
		return nil
	}

	var routes []*eskip.Route

	groupName := createGroupName(r.Id)
	decisionRoute := createDecisionRoute(r, groupName, len(backends))
	routes = append(routes, decisionRoute)

	memberRoutes := createMembers(r, groupName, backends)
	routes = append(routes, memberRoutes...)

	return routes
}
