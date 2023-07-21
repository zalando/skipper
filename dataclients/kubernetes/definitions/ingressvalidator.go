package definitions

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type IngressV1Validator struct {
	FiltersRegistry filters.Registry
}

func (igv *IngressV1Validator) Validate(item *IngressV1Item) error {
	var errs []error

	errs = append(errs, igv.validateFilterAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validatePredicateAnnotation(item.Metadata.Annotations))
	errs = append(errs, igv.validateRoutesAnnotation(item.Metadata.Annotations))

	return errors.Join(errs...)
}

func (igv *IngressV1Validator) validateFilterAnnotation(annotations map[string]string) error {
	var errs []error
	if filters, ok := annotations[IngressFilterAnnotation]; ok {
		parsedFilters, err := eskip.ParseFilters(filters)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err))
		}

		if igv.FiltersRegistry != nil {
			errs = append(errs, igv.validateFiltersNames(parsedFilters))
		}
	}

	return errorsJoin(errs...)
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
	var errs []error
	if routes, ok := annotations[IngressRoutesAnnotation]; ok {
		parsedRoutes, err := eskip.Parse(routes)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err))
		}

		if igv.FiltersRegistry != nil {
			for _, r := range parsedRoutes {
				errs = append(errs, igv.validateFiltersNames(r.Filters))
			}
		}
	}

	return errorsJoin(errs...)
}

func (igv *IngressV1Validator) validateFiltersNames(filters []*eskip.Filter) error {
	for _, f := range filters {
		if _, ok := igv.FiltersRegistry[f.Name]; !ok {
			return fmt.Errorf("filter \"%s\" is not supported", f.Name)
		}
	}
	return nil
}
