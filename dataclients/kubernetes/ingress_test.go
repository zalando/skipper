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
	)
}

func TestIngressV1AnnotationConfig(t *testing.T) {
	kubernetestest.FixturesToTest(t,
		"testdata/ingressV1/annotation-predicates",
		"testdata/ingressV1/annotation-filters",
		"testdata/ingressV1/annotation-backends",
	)
}
