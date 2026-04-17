package kubernetes_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestIngressV1Fixtures(t *testing.T) {
	kubernetestest.FixturesToTest(
		t,
		"testdata/ingressV1/named-ports",
		"testdata/ingressV1/ingress-data",
		"testdata/ingressV1/eastwest",
		"testdata/ingressV1/eastwestrange",
		"testdata/ingressV1/service-ports",
		"testdata/ingressV1/service-ports-endpointslices",
		"testdata/ingressV1/external-name",
		"testdata/ingressV1/tls",
		"testdata/ingressV1/traffic",
		"testdata/ingressV1/traffic-segment",
		"testdata/ingressV1/loadbalancer-algorithm",
		"testdata/ingressV1/zone-aware-traffic",
	)
}

func TestIngressV1AnnotationConfig(t *testing.T) {
	kubernetestest.FixturesToTest(t,
		"testdata/ingressV1/annotation-predicates",
		"testdata/ingressV1/annotation-filters",
		"testdata/ingressV1/annotation-backends",
	)
}

// TestIngressV1AnnotateFromAnnotations validates that Kubernetes resource
// annotations and labels configured via AnnotationsToRouteAnnotations /
// LabelsToRouteAnnotations are injected as annotate() filters into the
// generated route's filter chain. This is the Kubernetes-side companion to
// the OIDC profile e2e tests: the annotate() filters set values that profile
// templates can read via {{index .Annotations "key"}}.
func TestIngressV1AnnotateFromAnnotations(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/ingressV1/annotate-from-annotations")
}
