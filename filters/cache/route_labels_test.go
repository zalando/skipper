package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteLabel(t *testing.T) {
	tests := map[string]string{
		"catalogue__en":  "catalogue__en",
		"kube_namespace": "kube_namespace",
		"plain_route":    "plain_route",
		"":               "unknown",
	}
	for routeID, want := range tests {
		assert.Equal(t, want, routeLabel(routeID), routeID)
	}
}
