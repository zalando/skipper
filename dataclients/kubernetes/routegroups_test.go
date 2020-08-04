package kubernetes_test

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestRouteGroupExamples(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/examples")
}

func TestRouteGroupConvert(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/convert")
}

func TestRouteGroupClusterState(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/cluster-state")
}

func TestRouteGroupTraffic(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/traffic")
}

func TestRouteGroupEastWest(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/east-west")
}

func TestRouteGroupHTTPSRedirect(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/https-redirect")
}

func TestRouteGroupDefaultFilters(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/default-filters")
}

func TestRouteGroupWithIngress(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/with-ingress")
}

func TestRouteGroupTracingTag(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/tracing-tag")
}
