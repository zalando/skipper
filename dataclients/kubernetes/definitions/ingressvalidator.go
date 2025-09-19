package definitions

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type IngressV1Validator struct {
	FilterRegistry filters.Registry
	PredicateSpecs []routing.PredicateSpec
	Metrics        metrics.Metrics
}

// check if IngressV1Validator implements the interface
var _ Validator[*IngressV1Item] = &IngressV1Validator{}

func (igv *IngressV1Validator) Validate(item *IngressV1Item) error {
	var errs []error

	errs = append(errs, igv.validateFilterAnnotation(item))
	errs = append(errs, igv.validatePredicateAnnotation(item))
	errs = append(errs, igv.validateRoutesAnnotation(item))

	return errors.Join(errs...)
}

func (igv *IngressV1Validator) validateFilterAnnotation(item *IngressV1Item) error {
	if filters, ok := item.Metadata.Annotations[IngressFilterAnnotation]; ok {
		filters, err := eskip.ParseFilters(filters)
		if err != nil {
			err = fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
		}
		// We can add a flag to enable/disable this advance validation
		// ingress and routegroup can have different flag values
		validateFilters(ResourceContext{
			Namespace:    item.Metadata.Namespace,
			Name:         item.Metadata.Name,
			ResourceType: ResourceTypeIngress,
		}, igv.FilterRegistry, filters)
		return err
	}

	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(item *IngressV1Item) error {
	if predicates, ok := item.Metadata.Annotations[IngressPredicateAnnotation]; ok {
		predicates, err := eskip.ParsePredicates(predicates)
		if err != nil {
			err = fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
		}
		validatePredicates(ResourceContext{
			Namespace:    item.Metadata.Namespace,
			Name:         item.Metadata.Name,
			ResourceType: ResourceTypeIngress,
		}, igv.PredicateSpecs, predicates)
		return err
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(item *IngressV1Item) error {
	if routes, ok := item.Metadata.Annotations[IngressRoutesAnnotation]; ok {
		routes, err := eskip.Parse(routes)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
		}
		routingOptions := &routing.Options{
			FilterRegistry: igv.FilterRegistry,
			Predicates:     igv.PredicateSpecs,
			Metrics:        igv.Metrics,
		}
		// We can add a flag to enable/disable this advance validation
		// ingress and routegroup can have different flag values
		for _, r := range routes {
			_, err := routing.ValidateRoute(routingOptions, r)
			if err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
			}
		}
	}
	return nil
}
