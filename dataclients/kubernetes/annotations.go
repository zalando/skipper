package kubernetes

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type AnnotationPredicates struct {
	Key        string
	Value      string
	Predicates []*eskip.Predicate
}

type AnnotationFilters struct {
	Key     string
	Value   string
	Filters []*eskip.Filter
}

const (
	applicationLabelKey      = "application"
	applicationAnnotationKey = "Application"
)

func appendAnnotationPredicates(annotationPredicates []AnnotationPredicates, annotations map[string]string, r *eskip.Route) {
	for _, ap := range annotationPredicates {
		if objAnnotationVal, ok := annotations[ap.Key]; ok && ap.Value == objAnnotationVal {
			// since this annotation is managed by skipper operator, we can safely assume that the predicate is valid
			// and we can append it to the route
			r.Predicates = append(r.Predicates, ap.Predicates...)
		}
	}
}

func appendAnnotationFilters(annotationFilters []AnnotationFilters, annotations map[string]string, r *eskip.Route) {
	for _, af := range annotationFilters {
		if objAnnotationVal, ok := annotations[af.Key]; ok && af.Value == objAnnotationVal {
			r.Filters = append(r.Filters, af.Filters...)
		}
	}
}

func applicationAnnotationFilter(labels map[string]string) *eskip.Filter {
	application, ok := labels[applicationLabelKey]
	if !ok || application == "" {
		return nil
	}

	return &eskip.Filter{
		Name: filters.AnnotateName,
		Args: []interface{}{applicationAnnotationKey, application},
	}
}

func prependApplicationAnnotation(labels map[string]string, r *eskip.Route) {
	if f := applicationAnnotationFilter(labels); f != nil {
		r.Filters = append([]*eskip.Filter{f}, r.Filters...)
	}
}
