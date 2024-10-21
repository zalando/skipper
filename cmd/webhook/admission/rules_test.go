package admission

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleAdmitter(t *testing.T) {
	if testing.Verbose() {
		log.SetLevel(log.DebugLevel)
	}

	ra, err := NewRuleAdmitterFrom("testdata/rules/rules.yaml")
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		input   string
		allowed bool
		message string
	}{
		{
			name:    "allowed ingress",
			input:   "testdata/rules/ingress-allowed.yaml",
			allowed: true,
		},
		{
			name:    "rejected ingress",
			input:   "testdata/rules/ingress-rejected.yaml",
			allowed: false,
			message: `Missing application label, see https://example.test/reference/labels-selectors/#application, ` +
				`zalando.org/skipper-filter: oauthTokeninfoAnyScope filter uses "uid" scope, see https://opensource.zalando.com/skipper/reference/filters/#oauthtokeninfoanyscope, ` +
				`zalando.org/skipper-routes: oauthTokeninfoAnyScope filter uses "uid" scope, see https://opensource.zalando.com/skipper/reference/filters/#oauthtokeninfoanyscope, ` +
				`Ingress rules must use alias.cluster-domain.test cluster domain`,
		},
		{
			name:    "allowed routegroup",
			input:   "testdata/rules/routegroup-allowed.yaml",
			allowed: true,
		},
		{
			name:    "rejected routegroup",
			input:   "testdata/rules/routegroup-rejected.yaml",
			allowed: false,
			message: `Missing application label, see https://example.test/reference/labels-selectors/#application, ` +
				`oauthTokeninfoAnyScope filter uses "uid" scope, see https://opensource.zalando.com/skipper/reference/filters/#oauthtokeninfoanyscope, ` +
				`RouteGroup must use alias.cluster-domain.test cluster domain`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := os.ReadFile(tc.input)
			require.NoError(t, err)

			y, err := yaml.YAMLToJSON(input)
			require.NoError(t, err)

			request := &admissionRequest{
				UID:    "test-uid",
				Object: json.RawMessage(y),
			}

			resp, err := ra.admit(request)
			require.NoError(t, err)
			assert.Equal(t, request.UID, resp.UID)
			assert.Equal(t, tc.allowed, resp.Allowed)

			if tc.message != "" {
				require.NotNil(t, resp.Result)
				assert.Equal(t, tc.message, resp.Result.Message)
			}
		})
	}
}
