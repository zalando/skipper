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

var defaultIngressV1Validator = IngressV1Validator{}

func ValidateIngressV1(item *IngressV1Item) error {
	return defaultIngressV1Validator.validate(item)
}

func ValidateIngressesV1(ingressList IngressV1List) error {
	var errs []error
	for _, i := range ingressList.Items {
		err := ValidateIngressV1(i)
		if err != nil {
			name := i.Metadata.Name
			namespace := i.Metadata.Namespace
			errs = append(errs, fmt.Errorf("%s/%s: %w", name, namespace, err))
		}
	}
	return errorsJoin(errs...)
}

func (IngressV1Validator) validate(item *IngressV1Item) error {
	var errs []error

	errs = append(errs, defaultIngressV1Validator.validateFilterAnnotation(item.Metadata.Annotations))
	errs = append(errs, defaultIngressV1Validator.validatePredicateAnnotation(item.Metadata.Annotations))
	errs = append(errs, defaultIngressV1Validator.validateRoutesAnnotation(item.Metadata.Annotations))

	return errorsJoin(errs...)
}

func (IngressV1Validator) validateFilterAnnotation(annotations map[string]string) error {
	if filters, ok := annotations[skipperfilterAnnotationKey]; ok {
		_, err := eskip.ParseFilters(filters)
		return err
	}
	return nil
}

func (IngressV1Validator) validatePredicateAnnotation(annotations map[string]string) error {
	if predicates, ok := annotations[skipperpredicateAnnotationKey]; ok {
		_, err := eskip.ParsePredicates(predicates)
		return err
	}
	return nil
}

func (IngressV1Validator) validateRoutesAnnotation(annotations map[string]string) error {
	if routes, ok := annotations[skipperRoutesAnnotationKey]; ok {
		_, err := eskip.Parse(routes)
		return err
	}
	return nil
}
