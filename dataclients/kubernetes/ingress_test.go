package kubernetes_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestIngressFixtures(t *testing.T) {
	kubernetestest.FixturesToTest(
		t,
		"testdata/ingress/named-ports",
		"testdata/ingress/ingress-data",
		"testdata/ingress/eastwest",
		"testdata/ingress/eastwestrange",
		"testdata/ingress/service-ports",
		"testdata/ingress/external-name",
	)
}

func TestIngressV1Fixtures(t *testing.T) {
	kubernetestest.FixturesToTest(
		t,
		"testdata/ingressV1/named-ports",
		"testdata/ingressV1/ingress-data",
		"testdata/ingressV1/eastwest",
		"testdata/ingressV1/eastwestrange",
		"testdata/ingressV1/service-ports",
		"testdata/ingressV1/external-name",
	)
}
