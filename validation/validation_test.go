package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/block"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/routing"
)

// admissionReview represents the Kubernetes admission webhook response structure
// used in tests. This avoids importing admission controller types directly.
type admissionReview struct {
	Response struct {
		Allowed bool `json:"allowed"`
		Status  struct {
			Message string `json:"message"`
		} `json:"status"`
	} `json:"response"`
}

func TestStartValidationRequiresTLS(t *testing.T) {
	patchLogrusExit(t)

	err := StartValidation(Options{Address: ":0", CertFile: "", KeyFile: "", EnableAdvancedValidation: false}, routing.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires TLS")
}

func TestValidationHandlers(t *testing.T) {
	testCases := []struct {
		name                     string
		path                     string
		payload                  map[string]any
		expectedAllowed          bool
		expectedMessage          string
		expectedInvalidRouteKeys []string
	}{
		{
			name: "routegroup filter validation success",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{`blockContent("abc")`}},
				}
			}),
			expectedAllowed: true,
		},
		{
			name: "routegroup filter validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{"blockContent()"}},
				}
			}),
			expectedMessage: "invalid filter parameters",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeRouteGroup, "ns-test", "rg-test", "filters", "blockContent", "invalid_filter_params"),
			},
		},
		{
			name: "routegroup unknown filter validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{"unknownFilter()"}},
				}
			}),
			expectedMessage: `filter "unknownFilter" not found`,
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeRouteGroup, "ns-test", "rg-test", "filters", "unknownFilter", "unknown_filter"),
			},
		},
		{
			name: "routegroup predicate validation success",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{`Methods("GET")`}},
				}
			}),
			expectedAllowed: true,
		},
		{
			name: "routegroup predicate validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"Methods()"}},
				}
			}),
			expectedMessage: "at least one method should be specified",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeRouteGroup, "ns-test", "rg-test", "predicates", "Methods", "invalid_predicate_params"),
			},
		},
		{
			name: "routegroup unknown predicate validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"UnknownPredicate()"}},
				}
			}),
			expectedMessage: `predicate "UnknownPredicate" not found`,
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeRouteGroup, "ns-test", "rg-test", "predicates", "UnknownPredicate", "unknown_predicate"),
			},
		},
		{
			name: "routegroup backend validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["backends"] = []map[string]any{
					{"name": "backend-1", "type": "network", "address": "example.com"},
				}
			}),
			expectedMessage: "backend address",
		},
		{
			name: "ingress predicate annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = `Methods("GET")`
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress predicate annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = "Methods()"
			}),
			expectedMessage: "at least one method should be specified",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "predicates", "Methods", "invalid_predicate_params"),
			},
		},
		{
			name: "ingress unknown predicate annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = "UnknownPredicate()"
			}),
			expectedMessage: `predicate "UnknownPredicate" not found`,
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "predicates", "UnknownPredicate", "unknown_predicate"),
			},
		},
		{
			name: "ingress filter annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = `blockContent("abc")`
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress filter annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "blockContent()"
			}),
			expectedMessage: "invalid filter parameters",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "filters", "blockContent", "invalid_filter_params"),
			},
		},
		{
			name: "ingress unknown filter annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "unknownFilter()"
			}),
			expectedMessage: `filter "unknownFilter" not found`,
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "filters", "unknownFilter", "unknown_filter"),
			},
		},
		{
			name: "ingress route validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = `r1: * -> blockContent("abc") -> "https://example.org"`
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress route filter validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = `r1: * -> blockContent() -> "https://example.org"`
			}),
			expectedMessage: "invalid filter parameters",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "route", "r1", "invalid_filter_params"),
			},
		},
		{
			name: "ingress route unknown filter validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = `r1: * -> unknownFilter() -> "https://example.org"`
			}),
			expectedMessage: `filter "unknownFilter" not found`,
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "route", "r1", "unknown_filter"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterRegistry := filters.Registry{}
			filterRegistry.Register(block.NewBlock(1024))

			predicateSpecs := []routing.PredicateSpec{
				methods.New(),
			}

			metricsMock := &metricstest.MockMetrics{}

			routingOptions := routing.Options{
				FilterRegistry: filterRegistry,
				Predicates:     predicateSpecs,
				Metrics:        metricsMock,
			}
			handler := newValidationHandler(true, routingOptions)

			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			resp := recorder.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var admissionResponse admissionReview

			require.NoError(t, json.NewDecoder(resp.Body).Decode(&admissionResponse))
			assert.Equal(t, tc.expectedAllowed, admissionResponse.Response.Allowed)
			if tc.expectedMessage != "" {
				assert.Contains(t, admissionResponse.Response.Status.Message, tc.expectedMessage)
			} else {
				assert.Empty(t, admissionResponse.Response.Status.Message)
			}

			actualInvalidRouteKeys := collectPositiveInvalidRouteKeys(metricsMock)
			if len(tc.expectedInvalidRouteKeys) == 0 {
				assert.Empty(t, actualInvalidRouteKeys)
			} else {
				assert.ElementsMatch(t, tc.expectedInvalidRouteKeys, actualInvalidRouteKeys)
			}
		})
	}
}

// editAndAddRoutePreProcessor is a test preprocessor that both transforms routes
// and adds additional routes, similar to OAuth Grant preprocessor behavior
type editAndAddRoutePreProcessor struct{}

func (p *editAndAddRoutePreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	result := make([]*eskip.Route, len(routes))

	// Transform existing routes (fix unknownFilter -> blockContent)
	for i, route := range routes {
		newRoute := *route // copy
		newRoute.Filters = make([]*eskip.Filter, len(route.Filters))
		for j, filter := range route.Filters {
			newFilter := *filter // copy
			if filter.Name == "unknownFilter" {
				newFilter.Name = "blockContent"
			}
			newRoute.Filters[j] = &newFilter
		}
		result[i] = &newRoute
	}

	// Add a callback route (similar to OAuth Grant)
	callbackRoute := &eskip.Route{
		Id: "__test_callback_route",
		Predicates: []*eskip.Predicate{{
			Name: "Path",
			Args: []any{"/.well-known/test-callback"},
		}},
		Filters: []*eskip.Filter{{
			Name: "blockContent",
			Args: []any{"callback"},
		}},
		BackendType: eskip.ShuntBackend,
	}

	return append(result, callbackRoute)
}

var _ routing.PreProcessor = &editAndAddRoutePreProcessor{}

func TestValidationWithPreProcessors(t *testing.T) {
	testCases := []struct {
		name              string
		path              string
		payload           map[string]any
		withPreProcessors bool
		expectedAllowed   bool
		expectedMessage   string
	}{
		{
			name: "routegroup with preprocessor - should succeed after transformation",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{`unknownFilter("test")`}}, // Will be transformed to blockContent("test")
				}
			}),
			withPreProcessors: true,
			expectedAllowed:   true,
		},
		{
			name: "routegroup without preprocessor - should fail with unknown filter",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{`unknownFilter("test")`}}, // Will fail without preprocessing
				}
			}),
			withPreProcessors: false,
			expectedAllowed:   false,
			expectedMessage:   `filter "unknownFilter" not found`,
		},
		{
			name: "ingress with preprocessor - should succeed after transformation",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = `unknownFilter("test")`
			}),
			withPreProcessors: true,
			expectedAllowed:   true,
		},
		{
			name: "ingress without preprocessor - should fail with unknown filter",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = `unknownFilter("test")`
			}),
			withPreProcessors: false,
			expectedAllowed:   false,
			expectedMessage:   `filter "unknownFilter" not found`,
		},
		{
			name: "ingress routes with preprocessor - should succeed",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = `r1: * -> unknownFilter("test") -> "https://example.org"`
			}),
			withPreProcessors: true,
			expectedAllowed:   true,
		},
		{
			name: "ingress routes without preprocessor - should fail",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = `r1: * -> unknownFilter("test") -> "https://example.org"`
			}),
			withPreProcessors: false,
			expectedAllowed:   false,
			expectedMessage:   `filter "unknownFilter" not found`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filterRegistry := filters.Registry{}
			filterRegistry.Register(block.NewBlock(1024))

			predicateSpecs := []routing.PredicateSpec{
				methods.New(),
			}

			metricsMock := &metricstest.MockMetrics{}

			routingOptions := routing.Options{
				FilterRegistry: filterRegistry,
				Predicates:     predicateSpecs,
				Metrics:        metricsMock,
			}

			if tc.withPreProcessors {
				// Use the custom preprocessor that both edits and adds routes
				routingOptions.PreProcessors = []routing.PreProcessor{
					&editAndAddRoutePreProcessor{},
				}
			}

			handler := newValidationHandler(true, routingOptions)

			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			resp := recorder.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var admissionResponse admissionReview

			require.NoError(t, json.NewDecoder(resp.Body).Decode(&admissionResponse))
			assert.Equal(t, tc.expectedAllowed, admissionResponse.Response.Allowed)
			if tc.expectedMessage != "" {
				assert.Contains(t, admissionResponse.Response.Status.Message, tc.expectedMessage)
			} else {
				assert.Empty(t, admissionResponse.Response.Status.Message)
			}
		})
	}
}

func patchLogrusExit(t *testing.T) {
	t.Helper()
	logger := log.StandardLogger()
	original := logger.ExitFunc
	logger.ExitFunc = func(int) {}
	t.Cleanup(func() {
		logger.ExitFunc = original
	})
}

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	log.SetLevel(log.WarnLevel)
}

func newRouteGroupPayload(modifier func(spec map[string]any)) map[string]any {
	spec := map[string]any{
		"backends": []map[string]any{
			{"name": "backend-1", "type": "network", "address": "https://example.org"},
		},
		"defaultBackends": []map[string]any{
			{"backendName": "backend-1"},
		},
		"routes": []map[string]any{},
	}
	if modifier != nil {
		modifier(spec)
	}

	return map[string]any{
		"request": map[string]any{
			"uid":       "req-uid",
			"name":      "rg-test",
			"namespace": "ns-test",
			"resource": map[string]any{
				"group":    "zalando.org",
				"version":  "v1",
				"resource": "routegroups",
			},
			"object": map[string]any{
				"metadata": map[string]any{
					"name":      "rg-test",
					"namespace": "ns-test",
				},
				"spec": spec,
			},
		},
	}
}

func newIngressPayload(modifier func(metadata map[string]any)) map[string]any {
	metadata := map[string]any{
		"name":        "ing-test",
		"namespace":   "ns-test",
		"annotations": map[string]any{},
	}
	if modifier != nil {
		modifier(metadata)
	}

	return map[string]any{
		"request": map[string]any{
			"uid":  "req-uid",
			"name": "ing-test",
			"object": map[string]any{
				"metadata": metadata,
			},
		},
	}
}

func collectPositiveInvalidRouteKeys(metricsMock *metricstest.MockMetrics) []string {
	gauges := make(map[string]float64)
	metricsMock.WithGauges(func(g map[string]float64) {
		for key, value := range g {
			gauges[key] = value
		}
	})

	var keys []string
	for key, value := range gauges {
		if value > 0 {
			keys = append(keys, key)
		}
	}

	return keys
}

func invalidRouteGaugeKey(resourceType definitions.ResourceType, namespace, name, subject, suffix, reason string) string {
	base := fmt.Sprintf("validation %q %s/%s %s", resourceType, namespace, name, subject)
	if suffix != "" {
		base = fmt.Sprintf("%s %s", base, suffix)
	}
	return fmt.Sprintf("route.invalid.%s..%s", base, reason)
}
