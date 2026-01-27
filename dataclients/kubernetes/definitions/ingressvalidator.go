package definitions

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
)

type IngressV1Validator struct {
	RoutingOptions           routing.Options
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
		parsedFilters, err := eskip.ParseFilters(filters)
		if err != nil {
			return fmt.Errorf("invalid %q annotation: %w", IngressFilterAnnotation, err)
		}
		if igv.EnableAdvancedValidation {
			if err := validateFilters(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeIngress,
			}, igv.RoutingOptions, parsedFilters); err != nil {
				return fmt.Errorf("invalid %q annotation: %w", IngressFilterAnnotation, err)
			}
		}
		return nil
	}

	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(item *IngressV1Item) error {
	if predicates, ok := item.Metadata.Annotations[IngressPredicateAnnotation]; ok {
		parsedPredicates, err := eskip.ParsePredicates(predicates)
		if err != nil {
			return fmt.Errorf("invalid %q annotation: %w", IngressPredicateAnnotation, err)
		}
		if igv.EnableAdvancedValidation {
			if err := validatePredicates(ResourceContext{
				Namespace:    item.Metadata.Namespace,
				Name:         item.Metadata.Name,
				ResourceType: ResourceTypeIngress,
			}, igv.RoutingOptions, parsedPredicates); err != nil {
				return fmt.Errorf("invalid %q annotation: %w", IngressPredicateAnnotation, err)
			}
		}
		return nil
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(item *IngressV1Item) error {
	if routes, ok := item.Metadata.Annotations[IngressRoutesAnnotation]; ok {
		var errs []error
		parsedRoutes, err := eskip.Parse(routes)
		if err != nil {
			return fmt.Errorf("invalid %q annotation: %w", IngressRoutesAnnotation, err)
		}
		if igv.EnableAdvancedValidation {
			for _, r := range parsedRoutes {
				if err := validateRoute(ResourceContext{
					Namespace:    item.Metadata.Namespace,
					Name:         item.Metadata.Name,
					ResourceType: ResourceTypeIngress,
				}, igv.RoutingOptions, r); err != nil {
					errs = append(errs, fmt.Errorf("invalid %q annotation: %w", IngressRoutesAnnotation, err))
				}
			}
		}
		return errors.Join(errs...)
	}
	return nil
}
