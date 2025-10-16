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
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/block"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/routing"
)

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
					{"filters": []string{"blockContent(\"abc\")"}},
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
			expectedMessage: "filter \"unknownFilter\" not found",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeRouteGroup, "ns-test", "rg-test", "filters", "unknownFilter", "unknown_filter"),
			},
		},
		{
			name: "routegroup predicate validation success",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"Methods(\"GET\")"}},
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
			expectedMessage: "predicate \"UnknownPredicate\" not found",
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
				annotations[definitions.IngressPredicateAnnotation] = "Methods(\"GET\")"
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
			expectedMessage: "predicate \"UnknownPredicate\" not found",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "predicates", "UnknownPredicate", "unknown_predicate"),
			},
		},
		{
			name: "ingress filter annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "blockContent(\"abc\")"
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
			expectedMessage: "filter \"unknownFilter\" not found",
			expectedInvalidRouteKeys: []string{
				invalidRouteGaugeKey(definitions.ResourceTypeIngress, "ns-test", "ing-test", "filters", "unknownFilter", "unknown_filter"),
			},
		},
		{
			name: "ingress route validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> blockContent(\"abc\") -> \"https://example.org\""
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress route filter validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> blockContent() -> \"https://example.org\""
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
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> unknownFilter() -> \"https://example.org\""
			}),
			expectedMessage: "filter \"unknownFilter\" not found",
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

			handler := newValidationHandler(true, filterRegistry, predicateSpecs, metricsMock)

			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			resp := recorder.Result()
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var review struct {
				Response struct {
					Allowed bool `json:"allowed"`
					Status  struct {
						Message string `json:"message"`
					} `json:"status"`
				} `json:"response"`
			}

			require.NoError(t, json.NewDecoder(resp.Body).Decode(&review))
			assert.Equal(t, tc.expectedAllowed, review.Response.Allowed)
			if tc.expectedMessage != "" {
				assert.Contains(t, review.Response.Status.Message, tc.expectedMessage)
			} else {
				assert.Empty(t, review.Response.Status.Message)
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
