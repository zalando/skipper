package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type ComprehensiveValidator struct {
	routingOptions *routing.Options
	metrics        metrics.Metrics
}

func NewComprehensiveValidator(filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec, mtr metrics.Metrics) *ComprehensiveValidator {
	routingOptions := &routing.Options{
		FilterRegistry: filterRegistry,
		Predicates:     predicateSpecs,
		Metrics:        mtr,
	}

	return &ComprehensiveValidator{
		routingOptions: routingOptions,
		metrics:        mtr,
	}
}

func (cv *ComprehensiveValidator) ValidateFilters(ctx ResourceContext, filters []*eskip.Filter) error {
	filterNames := make([]string, len(filters))
	for i, filter := range filters {
		filterNames[i] = filter.Name
	}
	filterNameString := strings.Join(filterNames, ",")
	routeId := fmt.Sprintf("validation %q %s/%s filters %s", ctx.ResourceType, ctx.Namespace, ctx.Name, filterNameString)
	route := &eskip.Route{
		Id:      routeId,
		Filters: filters,
	}

	_, err := routing.ValidateRoute(cv.routingOptions, route)
	if validationErr := cv.handleValidationError(err, routeId); validationErr != nil {
		return fmt.Errorf("filter validation failed: %w", validationErr)
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidatePredicates(ctx ResourceContext, predicates []*eskip.Predicate) error {
	predicateNames := make([]string, len(predicates))
	for i, predicate := range predicates {
		predicateNames[i] = predicate.Name
	}
	predicateNameString := strings.Join(predicateNames, ",")
	routeId := fmt.Sprintf("validation %q %s/%s predicates %s", ctx.ResourceType, ctx.Namespace, ctx.Name, predicateNameString)
	route := &eskip.Route{
		Id:         routeId,
		Predicates: predicates,
	}

	_, err := routing.ValidateRoute(cv.routingOptions, route)
	if validationErr := cv.handleValidationError(err, routeId); validationErr != nil {
		return fmt.Errorf("predicate validation failed: %w", validationErr)
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidateRoute(ctx ResourceContext, routes []*eskip.Route) error {
	for _, route := range routes {
		routeId := fmt.Sprintf("validation %q %s/%s %s", ctx.ResourceType, ctx.Namespace, ctx.Name, route.Id)
		// Create a copy with the validation route ID for metrics tracking
		validationRoute := *route
		validationRoute.Id = routeId

		_, err := routing.ValidateRoute(cv.routingOptions, &validationRoute)
		if validationErr := cv.handleValidationError(err, routeId); validationErr != nil {
			return fmt.Errorf("route validation failed for route %s: %w", route.Id, validationErr)
		}
	}
	return nil
}

func (cv *ComprehensiveValidator) ValidateBackend(ctx ResourceContext, backend string, backendType eskip.BackendType) error {
	_, _, err := routing.SplitBackend(backend, backendType, false)

	routeId := fmt.Sprintf("validation %q %s/%s backend %s", ctx.ResourceType, ctx.Namespace, ctx.Name, backend)

	return cv.handleValidationError(err, routeId)
}

func (cv *ComprehensiveValidator) handleValidationError(err error, routeId string) error {
	if err == nil {
		if cv.metrics != nil {
			cv.metrics.DeleteInvalidRoute(routeId)
		}
		return nil
	}

	if cv.metrics != nil {
		var defErr routing.InvalidDefinitionError
		reason := "other"
		if errors.As(err, &defErr) {
			reason = defErr.Code()
		}
		cv.metrics.SetInvalidRoute(routeId, reason)
	}
	return err
}
