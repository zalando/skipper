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
	FilterRegistry           filters.Registry
	PredicateSpecs           []routing.PredicateSpec
	Metrics                  metrics.Metrics
	EnableAdvancedValidation bool
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
		var errs []error
		parsedFilters, err := eskip.ParseFilters(filters)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err))
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if igv.EnableAdvancedValidation {
			if err := validateFilters(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeIngress,
			}, igv.FilterRegistry, igv.Metrics, parsedFilters); err != nil {
				errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err))
			}
		}
		return errors.Join(errs...)
	}

	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(item *IngressV1Item) error {
	if predicates, ok := item.Metadata.Annotations[IngressPredicateAnnotation]; ok {
		var errs []error
		parsedPredicates, err := eskip.ParsePredicates(predicates)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err))
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if igv.EnableAdvancedValidation {
			if err := validatePredicates(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeIngress,
			}, igv.PredicateSpecs, igv.Metrics, parsedPredicates); err != nil {
				errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err))
			}
		}
		return errors.Join(errs...)
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(item *IngressV1Item) error {
	if routes, ok := item.Metadata.Annotations[IngressRoutesAnnotation]; ok {
		var errs []error
		parsedRoutes, err := eskip.Parse(routes)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err))
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if igv.EnableAdvancedValidation {
			opts := routing.Options{
				FilterRegistry: igv.FilterRegistry,
				Predicates:     igv.PredicateSpecs,
			}
			for _, r := range parsedRoutes {
				if err := validateRoute(ResourceContext{
					Namespace:    item.Metadata.Namespace,
					Name:         item.Metadata.Name,
					ResourceType: ResourceTypeIngress,
				}, opts, igv.Metrics, r); err != nil {
					errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err))
				}
			}
		}
		return errors.Join(errs...)
	}
	return nil
}
