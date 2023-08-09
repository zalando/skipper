package definitions

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
)

type IngressV1Validator struct{}

func (igv *IngressV1Validator) Validate(item *IngressV1Item) error {
	var errs []error

	errs = append(errs, igv.validateFilterAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validatePredicateAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validateRoutesAnnotation(item.Metadata.Annotations))

	return errorsJoin(errs...)
}

func (igv *IngressV1Validator) validateFilterAnnotation(annotations map[string]string) error {
	if filters, ok := annotations[IngressFilterAnnotation]; ok {
		_, err := eskip.ParseFilters(filters)
		if err != nil {
			err = fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
		}
		return err
	}
	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(annotations map[string]string) error {
	if predicates, ok := annotations[IngressPredicateAnnotation]; ok {
		_, err := eskip.ParsePredicates(predicates)
		if err != nil {
			err = fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
		}
		return err
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(annotations map[string]string) error {
	if routes, ok := annotations[IngressRoutesAnnotation]; ok {
		_, err := eskip.Parse(routes)
		if err != nil {
			err = fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
		}
		return err
	}
	return nil
}
