package validation

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type EskipValidator interface {
	ValidateFilters(ctx ResourceContext, filters []*eskip.Filter) error
	ValidatePredicates(ctx ResourceContext, predicates []*eskip.Predicate) error
	ValidateRoute(ctx ResourceContext, routes []*eskip.Route) error
	ValidateBackend(ctx ResourceContext, backend string, backendType eskip.BackendType) error
}

type Runner interface {
	StartValidation(config Config, filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) error
}
