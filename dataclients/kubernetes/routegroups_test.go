package kubernetes

import "testing"

func TestRouteGroupFixtures(t *testing.T) {
	t.Run("examples", func(t *testing.T) {
		testFixtures(t, "fixtures/routegroups/examples")
	})
}
