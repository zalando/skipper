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

// injectAnnotateFilters prepends annotate(prefix+key, value) filters into r.Filters for each
// key in keysToInject that is present in annotations. prefix is prepended to the key used in
// the annotate() call (but not to the annotation lookup key). This makes Kubernetes resource
// annotation values accessible to downstream filters (e.g. oauthOidc* profile filters)
// via annotate.GetAnnotations(ctx).
func injectAnnotateFilters(annotations map[string]string, keysToInject []string, prefix string, r *eskip.Route) {
	var toAdd []*eskip.Filter
	for _, key := range keysToInject {
		if val, ok := annotations[key]; ok {
			toAdd = append(toAdd, &eskip.Filter{
				Name: filters.AnnotateName,
				Args: []interface{}{prefix + key, val},
			})
		}
	}
	if len(toAdd) == 0 {
		return
	}
	r.Filters = append(toAdd, r.Filters...)
}
