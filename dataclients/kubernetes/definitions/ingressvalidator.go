package definitions

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/validation"

	"github.com/zalando/skipper/eskip"
)

type IngressV1Validator struct {
	validator validation.EskipValidator
}

func NewIngressV1Validator(validator validation.EskipValidator) *IngressV1Validator {
	return &IngressV1Validator{
		validator: validator,
	}
}

func (igv *IngressV1Validator) ValidateFilters(ctx validation.ResourceContext, filters []*eskip.Filter) error {
	if igv.validator != nil {
		return igv.validator.ValidateFilters(ctx, filters)
	}
	return nil
}

func (igv *IngressV1Validator) ValidatePredicates(ctx validation.ResourceContext, predicates []*eskip.Predicate) error {
	if igv.validator != nil {
		return igv.validator.ValidatePredicates(ctx, predicates)
	}
	return nil
}

func (igv *IngressV1Validator) ValidateRoute(ctx validation.ResourceContext, routes []*eskip.Route) error {
	if igv.validator != nil {
		return igv.validator.ValidateRoute(ctx, routes)
	}
	return nil
}

func (igv *IngressV1Validator) ValidateBackend(ctx validation.ResourceContext, backend string, backendType eskip.BackendType) error {
	if igv.validator != nil {
		return igv.validator.ValidateBackend(ctx, backend, backendType)
	}
	return nil
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
		ctx := validation.ResourceContext{
			Namespace:    namespaceString(item.Metadata.Namespace),
			Name:         item.Metadata.Name,
			ResourceType: validation.ResourceTypeIngress,
		}
		if err := igv.ValidateFilters(ctx, filters); err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressFilterAnnotation, err)
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
		ctx := validation.ResourceContext{
			Namespace:    namespaceString(item.Metadata.Namespace),
			Name:         item.Metadata.Name,
			ResourceType: validation.ResourceTypeIngress,
		}
		if err := igv.ValidatePredicates(ctx, predicates); err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressPredicateAnnotation, err)
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
		ctx := validation.ResourceContext{
			Namespace:    namespaceString(item.Metadata.Namespace),
			Name:         item.Metadata.Name,
			ResourceType: validation.ResourceTypeIngress,
		}
		if err := igv.ValidateRoute(ctx, route); err != nil {
			return fmt.Errorf("invalid \"%s\" annotation: %w", IngressRoutesAnnotation, err)
		}
	}
	return nil
}
