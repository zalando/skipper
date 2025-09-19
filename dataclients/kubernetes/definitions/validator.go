package definitions

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
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

func validateFilters(resourceCtx ResourceContext, filterRegistry filters.Registry, filters []*eskip.Filter) error {
	routeId := fmt.Sprintf("validation %q %s/%s filters", resourceCtx.ResourceType, resourceCtx.Namespace, resourceCtx.Name)
	route := &eskip.Route{
		Id:      routeId,
		Filters: filters,
	}

	_, err := routing.ValidateRoute(&routing.Options{FilterRegistry: filterRegistry}, route)
	return err
}

func validatePredicates(resourceCtx ResourceContext, predicateSpecs []routing.PredicateSpec, predicates []*eskip.Predicate) error {
	routeId := fmt.Sprintf("validation %q %s/%s predicates", resourceCtx.ResourceType, resourceCtx.Namespace, resourceCtx.Name)
	route := &eskip.Route{
		Id:         routeId,
		Predicates: predicates,
	}

	_, err := routing.ValidateRoute(&routing.Options{Predicates: predicateSpecs}, route)
	return err
}

func ValidateBackend(ctx ResourceContext, backend string, backendType eskip.BackendType) error {
	_, _, err := routing.SplitBackend(backend, backendType, false)

	return err
}
