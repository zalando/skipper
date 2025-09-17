package definitions

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/validation"

	"github.com/zalando/skipper/eskip"
)

type IngressV1Validator struct {
	validation.EskipValidator
}

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
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
		}
		if igv.EskipValidator != nil {
			ctx := validation.ResourceContext{
				Namespace:    namespaceString(item.Metadata.Namespace),
				Name:         item.Metadata.Name,
				ResourceType: validation.ResourceTypeIngress,
			}
			if err := igv.EskipValidator.ValidateFilters(ctx, filters); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
			}
		}
	}
	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(item *IngressV1Item) error {
	if predicates, ok := item.Metadata.Annotations[IngressPredicateAnnotation]; ok {
		predicates, err := eskip.ParsePredicates(predicates)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
		}
		if igv.EskipValidator != nil {
			ctx := validation.ResourceContext{
				Namespace:    namespaceString(item.Metadata.Namespace),
				Name:         item.Metadata.Name,
				ResourceType: validation.ResourceTypeIngress,
			}
			if err := igv.EskipValidator.ValidatePredicates(ctx, predicates); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
			}
		}
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(item *IngressV1Item) error {
	if routes, ok := item.Metadata.Annotations[IngressRoutesAnnotation]; ok {
		route, err := eskip.Parse(routes)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
		}
		if igv.EskipValidator != nil {
			ctx := validation.ResourceContext{
				Namespace:    namespaceString(item.Metadata.Namespace),
				Name:         item.Metadata.Name,
				ResourceType: validation.ResourceTypeIngress,
			}
			if err := igv.EskipValidator.ValidateRoute(ctx, route); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
			}
		}
	}
	return nil
}
