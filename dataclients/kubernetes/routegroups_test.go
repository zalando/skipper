package kubernetes

import "testing"

func TestRouteGroupExamples(t *testing.T) {
	testFixtures(t, "testdata/routegroups/examples")
}

func TestRouteGroupConvert(t *testing.T) {
	testFixtures(t, "testdata/routegroups/convert")
}

func TestRouteGroupClusterState(t *testing.T) {
	testFixtures(t, "testdata/routegroups/cluster-state")
}

func TestRouteGroupValidation(t *testing.T) {
	testFixtures(t, "testdata/routegroups/validation")
}

func TestRouteGroupDefaultFilters(t *testing.T) {
	testFixtures(t, "testdata/routegroups/default-filters")
}

func TestRouteGroupWithIngress(t *testing.T) {
	testFixtures(t, "testdata/routegroups/with-ingress")
}
