package validation

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type ComprehensiveValidator struct {
	routingOptions *routing.Options
}

func NewComprehensiveValidator(filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) *ComprehensiveValidator {
	routingOptions := &routing.Options{
		FilterRegistry: filterRegistry,
		Predicates:     predicateSpecs,
	}

	return &ComprehensiveValidator{
		routingOptions: routingOptions,
	}
}

func (cv *ComprehensiveValidator) ValidateFilters(filters []*eskip.Filter) error {
	route := &eskip.Route{
		Id:      "validation-route",
		Filters: filters,
	}

	_, err := routing.ValidateRoute(cv.routingOptions, route)
	if err != nil {
		return fmt.Errorf("filter validation failed: %w", err)
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidatePredicates(predicates []*eskip.Predicate) error {
	route := &eskip.Route{
		Id:         "validation-route",
		Predicates: predicates,
	}

	_, err := routing.ValidateRoute(cv.routingOptions, route)
	if err != nil {
		return fmt.Errorf("predicate validation failed: %w", err)
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidateRoute(routes []*eskip.Route) error {
	for _, route := range routes {
		_, err := routing.ValidateRoute(cv.routingOptions, route)
		if err != nil {
			return fmt.Errorf("backend validation failed for route %s: %w", route.Id, err)
		}
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidateBackend(backend string, backendType eskip.BackendType) error {
	_, _, err := routing.SplitBackend(backend, backendType, false)
	return err
}
