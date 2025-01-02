package kubernetes

import (
	"github.com/zalando/skipper/eskip"
)

type AnnotationPredicates struct {
	Key        string
	Value      string
	Predicates []*eskip.Predicate
}

func addAnnotationPredicates(annotationPredicates []AnnotationPredicates, annotations map[string]string, r *eskip.Route) {
	for _, ap := range annotationPredicates {
		if objAnnotationVal, ok := annotations[ap.Key]; ok && ap.Value == objAnnotationVal {
			// since this annotation is managed by skipper operator, we can safely assume that the predicate is valid
			// and we can append it to the route
			r.Predicates = append(r.Predicates, ap.Predicates...)
		}
	}
}
