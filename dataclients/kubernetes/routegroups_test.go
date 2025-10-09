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

func TestRouteGroupTrafficSegment(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/traffic-segment")
}

func TestRouteGroupEastWest(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/east-west")
}

func TestRouteGroupEastWestRange(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/east-west-range")
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

func TestRouteGroupExternalName(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/external-name")
}

func TestRouteGroupDefaultLoadBalancerAlgorithm(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/loadbalancer-algorithm")
}

func TestRouteGroupTLS(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/tls")
}

func TestRouteGroupAnnotationConfig(t *testing.T) {
	kubernetestest.FixturesToTest(t,
		"testdata/routegroups/annotation-predicates",
		"testdata/routegroups/annotation-filters")
}

func TestRouteGroupBackends(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/routegroups/backends")
}
