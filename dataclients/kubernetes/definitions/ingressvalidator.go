package definitions

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
)

const (
	skipperfilterAnnotationKey    = "zalando.org/skipper-filter"
	skipperpredicateAnnotationKey = "zalando.org/skipper-predicate"
	skipperRoutesAnnotationKey    = "zalando.org/skipper-routes"
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
	if filters, ok := annotations[skipperfilterAnnotationKey]; ok {
		_, err := eskip.ParseFilters(filters)
		if err != nil {
			err = fmt.Errorf("parsing \"%s\" annotation failed: %w", skipperfilterAnnotationKey, err)
		}
		return err
	}
	return nil
}

func (igv *IngressV1Validator) validatePredicateAnnotation(annotations map[string]string) error {
	if predicates, ok := annotations[skipperpredicateAnnotationKey]; ok {
		_, err := eskip.ParsePredicates(predicates)
		if err != nil {
			err = fmt.Errorf("parsing \"%s\" annotation failed: %w", skipperpredicateAnnotationKey, err)
		}
		return err
	}
	return nil
}

func (igv *IngressV1Validator) validateRoutesAnnotation(annotations map[string]string) error {
	if routes, ok := annotations[skipperRoutesAnnotationKey]; ok {
		_, err := eskip.Parse(routes)
		if err != nil {
			err = fmt.Errorf("parsing \"%s\" annotation failed: %w", skipperRoutesAnnotationKey, err)
		}
		return err
	}
	return nil
}
