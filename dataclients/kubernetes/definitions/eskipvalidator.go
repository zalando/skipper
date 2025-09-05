package definitions

import "github.com/zalando/skipper/eskip"

type ResourceContext struct {
	Namespace    string
	Name         string
	ResourceType string // "rg" or "ing"
}

type EskipValidator interface {
	ValidateFilters(ctx ResourceContext, filters []*eskip.Filter) error
	ValidatePredicates(ctx ResourceContext, predicates []*eskip.Predicate) error
	ValidateRoute(ctx ResourceContext, routes []*eskip.Route) error
	ValidateBackend(ctx ResourceContext, backend string, backendType eskip.BackendType) error
}
