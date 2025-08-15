package definitions

import "github.com/zalando/skipper/eskip"

type EskipValidator interface {
	ValidateFilters(filters []*eskip.Filter) error
	ValidatePredicates(predicates []*eskip.Predicate) error
	ValidateRoute(routes []*eskip.Route) error
	ValidateBackend(backend string, backendType eskip.BackendType) error
}
