package validation

import (
	"fmt"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type ComprehensiveValidator struct {
	filterRegistry filters.Registry
	predicateSpecs []routing.PredicateSpec
	routingOptions *routing.Options
}

func NewComprehensiveValidator(filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) *ComprehensiveValidator {
	routingOptions := &routing.Options{
		FilterRegistry: filterRegistry,
		Predicates:     predicateSpecs,
	}

	return &ComprehensiveValidator{
		filterRegistry: filterRegistry,
		predicateSpecs: predicateSpecs,
		routingOptions: routingOptions,
	}
}

func (cv *ComprehensiveValidator) ValidateRouteGroup(rg *definitions.RouteGroupItem) error {
	if err := definitions.ValidateRouteGroup(rg); err != nil {
		return fmt.Errorf("basic validation failed: %w", err)
	}

	routes := cv.convertRouteGroupToMinimalEskip(rg)
	return cv.validateRoutes(routes)
}

func (cv *ComprehensiveValidator) ValidateIngress(ing *definitions.IngressV1Item) error {
	routes := cv.convertIngressToMinimalEskip(ing)
	return cv.validateRoutes(routes)
}

func (cv *ComprehensiveValidator) convertRouteGroupToMinimalEskip(rg *definitions.RouteGroupItem) []*eskip.Route {
	var routes []*eskip.Route

	for i, route := range rg.Spec.Routes {
		eskipRoute := &eskip.Route{
			Id:      fmt.Sprintf("%s_%d", rg.Metadata.Name, i),
			Backend: "http://mock-url.com",
		}

		for _, filterStr := range route.Filters {
			filters, err := eskip.ParseFilters(filterStr)
			if err != nil {
				continue
			}
			eskipRoute.Filters = append(eskipRoute.Filters, filters...)
		}

		for _, predicateStr := range route.Predicates {
			predicates, err := eskip.ParsePredicates(predicateStr)
			if err != nil {
				continue
			}
			eskipRoute.Predicates = append(eskipRoute.Predicates, predicates...)
		}

		routes = append(routes, eskipRoute)
	}

	return routes
}

func (cv *ComprehensiveValidator) convertIngressToMinimalEskip(ing *definitions.IngressV1Item) []*eskip.Route {
	var routes []*eskip.Route

	routesAnnotation := ing.Metadata.Annotations["zalando.org/skipper-routes"]
	if routesAnnotation != "" {
		parsed, err := eskip.Parse(routesAnnotation)
		if err == nil {
			routes = append(routes, parsed...)
		}
	}

	filtersAnnotation := ing.Metadata.Annotations["zalando.org/skipper-filter"]
	predicatesAnnotation := ing.Metadata.Annotations["zalando.org/skipper-predicate"]

	for i, rule := range ing.Spec.Rules {
		if rule.Http == nil {
			continue
		}

		for j := range rule.Http.Paths {
			eskipRoute := &eskip.Route{
				Id:      fmt.Sprintf("%s_%d_%d", ing.Metadata.Name, i, j),
				Backend: "http://example.com",
			}

			if filtersAnnotation != "" {
				filters, err := eskip.ParseFilters(filtersAnnotation)
				if err == nil {
					eskipRoute.Filters = append(eskipRoute.Filters, filters...)
				}
			}

			if predicatesAnnotation != "" {
				predicates, err := eskip.ParsePredicates(predicatesAnnotation)
				if err == nil {
					eskipRoute.Predicates = append(eskipRoute.Predicates, predicates...)
				}
			}

			routes = append(routes, eskipRoute)
		}

	}

	return routes
}

func (cv *ComprehensiveValidator) validateRoutes(routes []*eskip.Route) error {
	predicateMap := routing.MapPredicates(cv.predicateSpecs)

	for _, route := range routes {
		_, err := routing.ProcessRouteDef(cv.routingOptions, predicateMap, route)
		if err != nil {
			return fmt.Errorf("route validation failed for route %s: %w", route.Id, err)
		}
	}

	return nil
}
