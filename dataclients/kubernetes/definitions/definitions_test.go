package definitions_test

import (
	"os"
	"strings"
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
	data, err := os.ReadFile("testdata/errorwrapdata/routegroups.json")
	require.NoError(t, err)

	logs, err := os.ReadFile("testdata/errorwrapdata/errors.log")
	require.NoError(t, err)

	logsString := strings.TrimSuffix(string(logs), "\n")

	rgl, err := definitions.ParseRouteGroupsJSON(data)
	require.NoError(t, err)

	err = definitions.ValidateRouteGroups(&rgl)

	assert.EqualError(t, err, logsString)
}
