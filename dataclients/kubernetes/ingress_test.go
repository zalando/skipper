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
