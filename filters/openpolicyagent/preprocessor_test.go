package openpolicyagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

// TestPreProcessorBundleExtraction tests the bundle extraction logic without dependencies
func TestPreProcessorBundleExtraction(t *testing.T) {
	registry, err := NewOpenPolicyAgentRegistry(WithPreloadingEnabled(true), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err, "Expected no error creating OpenPolicyAgentRegistry")
	defer registry.Close()

	preprocessor := registry.NewPreProcessor().(*opaPreProcessor)

	testCases := []struct {
		name     string
		routes   []*eskip.Route
		expected []string
	}{
		{
			name: "multiple different bundles",
			routes: []*eskip.Route{
				{
					Id: "route1",
					Filters: []*eskip.Filter{
						{Name: "opaAuthorizeRequest", Args: []interface{}{"bundle1"}},
					},
				},
				{
					Id: "route2",
					Filters: []*eskip.Filter{
						{Name: "opaServeResponse", Args: []interface{}{"bundle2"}},
					},
				},
			},
			expected: []string{"bundle1", "bundle2"},
		},
		{
			name: "duplicate bundles should be deduplicated",
			routes: []*eskip.Route{
				{
					Id: "route1",
					Filters: []*eskip.Filter{
						{Name: "opaAuthorizeRequest", Args: []interface{}{"bundle1"}},
					},
				},
				{
					Id: "route2",
					Filters: []*eskip.Filter{
						{Name: "opaAuthorizeRequest", Args: []interface{}{"bundle1"}},
					},
				},
			},
			expected: []string{"bundle1"},
		},
		{
			name: "non-opa filters should be ignored",
			routes: []*eskip.Route{
				{
					Id: "route1",
					Filters: []*eskip.Filter{
						{Name: "requestHeader", Args: []interface{}{"X-Test", "value"}},
						{Name: "opaAuthorizeRequest", Args: []interface{}{"bundle1"}},
						{Name: "responseHeader", Args: []interface{}{"X-Response", "value"}},
					},
				},
			},
			expected: []string{"bundle1"},
		},
		{
			name: "no opa filters should return empty",
			routes: []*eskip.Route{
				{
					Id: "route1",
					Filters: []*eskip.Filter{
						{Name: "requestHeader", Args: []interface{}{"X-Test", "value"}},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "opa filters without args should be ignored",
			routes: []*eskip.Route{
				{
					Id: "route1",
					Filters: []*eskip.Filter{
						{Name: "opaAuthorizeRequest", Args: []interface{}{}},
					},
				},
			},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := preprocessor.extractOpaBundleRequests(tc.routes)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestPreprocessorRoutesUnchanged verifies the preprocessor doesn't modify routes
func TestPreprocessorRoutesUnchanged(t *testing.T) {
	registry, err := NewOpenPolicyAgentRegistry(WithPreloadingEnabled(true), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err, "Expected no error creating OpenPolicyAgentRegistry")
	defer registry.Close()

	preprocessor := registry.NewPreProcessor()

	originalRoutes := []*eskip.Route{
		{
			Id: "test-route",
			Filters: []*eskip.Filter{
				{Name: "requestHeader", Args: []interface{}{"X-Test", "value"}},
				{Name: "opaAuthorizeRequest", Args: []interface{}{"test-bundle"}},
			},
		},
	}

	// Deep copy to compare
	expectedRoutes := make([]*eskip.Route, len(originalRoutes))
	for i, route := range originalRoutes {
		expectedRoutes[i] = &eskip.Route{
			Id:      route.Id,
			Filters: make([]*eskip.Filter, len(route.Filters)),
		}
		copy(expectedRoutes[i].Filters, route.Filters)
	}

	// Process the routes
	result := preprocessor.Do(originalRoutes)

	// Verify routes are unchanged
	assert.Equal(t, expectedRoutes, result)
	assert.Equal(t, expectedRoutes, originalRoutes) // Original should also be unchanged
}

// TestStartScheduledPreventsDuplicateEnqueue tests that enqueueInstancesSequential
// will not allocate multiple queue slots to the same instance start task
func TestStartScheduledPreventsDuplicateEnqueue(t *testing.T) {
	registry, err := NewOpenPolicyAgentRegistry(WithPreloadingEnabled(true),
		WithBackgroundTaskBufferSize(2), WithOpenPolicyAgentInstanceConfig(WithConfigTemplate([]byte(""))))
	require.NoError(t, err, "Expected no error creating OpenPolicyAgentRegistry")
	defer registry.Close()
	preprocessor := registry.NewPreProcessor().(*opaPreProcessor)

	//in the real would scenario this will happen only with multiple route updates combined
	preprocessor.enqueueInstancesSequential([]string{"bundle1", "bundle1", "bundle1", "bundle1", "bundle1", "bundle2"})
	instance1, _ := registry.GetOrStartInstance("bundle1")
	instance2, _ := registry.GetOrStartInstance("bundle2")

	assert.True(t, instance1.StartScheduled(), "Expected StartScheduled to be true")
	assert.True(t, instance2.StartScheduled(), "Expected StartScheduled to be true")
}
