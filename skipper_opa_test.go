//go:build opa

package skipper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/openpolicyagent/defaults"
	"github.com/zalando/skipper/metrics"
)

func TestNewOpenPolicyAgentWithBuildTag(t *testing.T) {
	configTemplate := filepath.Join(t.TempDir(), "opaconfig.yaml")
	require.NoError(t, os.WriteFile(configTemplate, []byte("{}"), 0o600))

	for _, tc := range []struct {
		name              string
		options           Options
		wantFilters       []string
		wantPreProcessors int
	}{
		{
			name: "disabled",
		},
		{
			name:    "enabled",
			options: Options{EnableOpenPolicyAgent: true},
			wantFilters: []string{
				filters.OpaAuthorizeRequestName,
				filters.OpaAuthorizeRequestWithBodyName,
				filters.OpaServeResponseName,
				filters.OpaServeResponseWithReqBodyName,
			},
		},
		{
			name:    "enabled with preloading",
			options: Options{EnableOpenPolicyAgent: true, EnableOpenPolicyAgentPreloading: true},
			wantFilters: []string{
				filters.OpaAuthorizeRequestName,
				filters.OpaAuthorizeRequestWithBodyName,
				filters.OpaServeResponseName,
				filters.OpaServeResponseWithReqBodyName,
			},
			wantPreProcessors: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.options.OpenPolicyAgentConfigTemplate = configTemplate
			tc.options.OpenPolicyAgentCleanerInterval = defaults.CleanIdlePeriod
			tc.options.OpenPolicyAgentStartupTimeout = defaults.OpaStartupTimeout
			tc.options.OpenPolicyAgentControlLoopInterval = defaults.ControlLoopInterval
			tc.options.OpenPolicyAgentControlLoopMaxJitter = defaults.ControlLoopMaxJitter
			tc.options.OpenPolicyAgentMaxRequestBodySize = defaults.MaxRequestBodySize
			tc.options.OpenPolicyAgentMaxMemoryBodyParsing = defaults.MaxMemoryBodyParsing
			tc.options.OpenPolicyAgentRequestBodyBufferSize = defaults.RequestBodyBufferSize
			opa, err := newOpenPolicyAgent(&tc.options, &mocktracer.MockTracer{}, metrics.NewMetrics(metrics.Options{}))
			require.NoError(t, err)
			defer opa.close()

			var names []string
			for _, spec := range opa.filters {
				names = append(names, spec.Name())
			}
			assert.Equal(t, tc.wantFilters, names)
			assert.Len(t, opa.preProcessors, tc.wantPreProcessors)
			if tc.options.EnableOpenPolicyAgent {
				assert.Len(t, opa.postProcessors, 1)
			} else {
				assert.Empty(t, opa.postProcessors)
			}
		})
	}
}
