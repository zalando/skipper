package kubernetes_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestMixedFixtures(t *testing.T) {
	kubernetestest.FixturesToTest(t,
		"testdata/mixed",
	)
}
