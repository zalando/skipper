package kubernetes_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestIngressFixtures(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/ingress/named-ports")
	kubernetestest.FixturesToTest(t, "testdata/ingress/ingress-data")
}
