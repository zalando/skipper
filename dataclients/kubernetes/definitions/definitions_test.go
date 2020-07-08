package definitions_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestRouteGroupValidation(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/validation")
}
