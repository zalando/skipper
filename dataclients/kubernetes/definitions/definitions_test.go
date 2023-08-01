package definitions_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestRouteGroupValidation(t *testing.T) {
	kubernetestest.FixturesToTest(t, "testdata/validation")
}

func TestValidateRouteGroups(t *testing.T) {
	data, err := os.ReadFile("testdata/errorwrapdata/routegroups.yaml")
	require.NoError(t, err)

	rgl, err := definitions.ParseRouteGroupsYAML(data)
	require.NoError(t, err)

	err = definitions.ValidateRouteGroups(&rgl)

	assert.EqualError(t, err, "route group without name\nerror in route group default/rg1: route group without backend")
}
