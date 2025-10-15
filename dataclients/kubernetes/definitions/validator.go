package definitions

import (
	"fmt"
	"strings"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type ResourceType string

const (
	ResourceTypeRouteGroup ResourceType = "RouteGroup"
	ResourceTypeIngress    ResourceType = "Ingress"
)

type KubernetesResource interface {
	*RouteGroupItem | *IngressV1Item
}

type ResourceContext struct {
	Namespace    string
	Name         string
	ResourceType ResourceType
}

type Validator[R KubernetesResource] interface {
	Validate(resource R) error
}

func validateFilters(resourceCtx ResourceContext, filterRegistry filters.Registry, mtr metrics.Metrics, filters []*eskip.Filter) error {
	filterNames := make([]string, len(filters))
	for i, filter := range filters {
		filterNames[i] = filter.Name
	}

	routeId := buildValidationRouteID(resourceCtx, "filters", strings.Join(filterNames, ","))
	route := &eskip.Route{Id: routeId, Filters: filters}

	options := &routing.Options{FilterRegistry: filterRegistry, Metrics: mtr}
	_, err := routing.ValidateRoute(options, route)
	return routing.HandleValidationError(mtr, err, routeId)
}

func validatePredicates(resourceCtx ResourceContext, predicateSpecs []routing.PredicateSpec, mtr metrics.Metrics, predicates []*eskip.Predicate) error {
	predicateNames := make([]string, len(predicates))
	for i, predicate := range predicates {
		predicateNames[i] = predicate.Name
	}

	routeId := buildValidationRouteID(resourceCtx, "predicates", strings.Join(predicateNames, ","))
	route := &eskip.Route{Id: routeId, Predicates: predicates}

	options := &routing.Options{Predicates: predicateSpecs, Metrics: mtr}
	_, err := routing.ValidateRoute(options, route)
	return routing.HandleValidationError(mtr, err, routeId)
}

func validateRoute(resourceCtx ResourceContext, baseOptions routing.Options, mtr metrics.Metrics, route *eskip.Route) error {
	originalID := route.Id
	routeId := buildValidationRouteID(resourceCtx, "route", originalID)
	validationRoute := *route
	validationRoute.Id = routeId

	options := baseOptions
	options.Metrics = mtr

	_, err := routing.ValidateRoute(&options, &validationRoute)
	return routing.HandleValidationError(mtr, err, routeId)
}

func validateBackend(resourceCtx ResourceContext, backend string, backendType eskip.BackendType, mtr metrics.Metrics) error {
	routeId := buildValidationRouteID(resourceCtx, "backend", backend)

	_, _, err := routing.SplitBackend(backend, backendType, false)
	if err != nil {
		err = routing.WrapInvalidDefinitionReason("failed_backend_split", err)
	}

	if validationErr := routing.HandleValidationError(mtr, err, routeId); validationErr != nil {
		return fmt.Errorf("backend validation failed: %w", validationErr)
	}

	return nil
}

func buildValidationRouteID(resourceCtx ResourceContext, subject, suffix string) string {
	if suffix != "" {
		return fmt.Sprintf("validation %q %s/%s %s %s", resourceCtx.ResourceType, resourceCtx.Namespace, resourceCtx.Name, subject, suffix)
	}
	return fmt.Sprintf("validation %q %s/%s %s", resourceCtx.ResourceType, resourceCtx.Namespace, resourceCtx.Name, subject)
}
