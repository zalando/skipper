package kubernetes

import "testing"

func TestRouteGroupExamples(t *testing.T) {
	testFixtures(t, "testdata/routegroups/examples")
}

func TestRouteGroupValidation(t *testing.T) {
	testFixtures(t, "testdata/routegroups/validation")
}

func TestRouteGroupConvert(t *testing.T) {
	testFixtures(t, "testdata/routegroups/convert")
}

func TestRouteGroupClusterState(t *testing.T) {
	testFixtures(t, "testdata/routegroups/cluster-state")
}

func TestRouteGroupTraffic(t *testing.T) {
	testFixtures(t, "testdata/routegroups/traffic")
}

func TestRouteGroupEastWest(t *testing.T) {
	testFixtures(t, "testdata/routegroups/east-west")
}

func TestRouteGroupHTTPSRedirect(t *testing.T) {
	testFixtures(t, "testdata/routegroups/https-redirect")
}

func TestRouteGroupDefaultFilters(t *testing.T) {
	testFixtures(t, "testdata/routegroups/default-filters")
}

func TestRouteGroupWithIngress(t *testing.T) {
	testFixtures(t, "testdata/routegroups/with-ingress")
}
