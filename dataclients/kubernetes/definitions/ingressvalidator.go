package definitions

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/eskip"
)

type IngressV1Validator struct {
	EskipValidator
}

func (igv *IngressV1Validator) Validate(item *IngressV1Item) error {
	var errs []error

	errs = append(errs, igv.validateFilterAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validatePredicateAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validateRoutesAnnotation(item.Metadata.Annotations))

	return errors.Join(errs...)
}

func (igv *IngressV1Validator) validateFilterAnnotation(annotations map[string]string) error {
	if filters, ok := annotations[IngressFilterAnnotation]; ok {
		filters, err := eskip.ParseFilters(filters)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
		}
		if igv.EskipValidator != nil {
			if err := igv.EskipValidator.ValidateFilters(filters); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
			}
		}
	}
	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(annotations map[string]string) error {
	if predicates, ok := annotations[IngressPredicateAnnotation]; ok {
		predicates, err := eskip.ParsePredicates(predicates)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
		}
		if igv.EskipValidator != nil {
			if err := igv.EskipValidator.ValidatePredicates(predicates); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
			}
		}
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(annotations map[string]string) error {
	if routes, ok := annotations[IngressRoutesAnnotation]; ok {
		route, err := eskip.Parse(routes)
		if err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
		}
		if igv.EskipValidator != nil {
			if err := igv.EskipValidator.ValidateRoute(route); err != nil {
				return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
			}
		}
	}
	return nil
}
