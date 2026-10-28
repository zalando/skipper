package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
	"google.golang.org/protobuf/proto"
)

func TestPrometheusCacheL2(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prefix    string
		namespace string
	}{
		{name: "default", namespace: "skipper"},
		{name: "prefix", prefix: "gateway.", namespace: "gateway"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			p := metrics.NewPrometheus(metrics.Options{Prefix: tc.prefix, PrometheusRegistry: registry})
			p.IncStorageError("l2", "get")
			before, err := registry.Gather()
			require.NoError(t, err)
			legacy := make(map[string]*dto.MetricFamily)
			for _, family := range before {
				require.NotContains(t, family.GetName(), "_cache_l2_")
				legacy[family.GetName()] = proto.Clone(family).(*dto.MetricFamily)
			}

			p.IncCacheL2Operation("get", "response", "success")
			p.IncCacheL2Operation("get", "response", "success")
			p.IncCacheL2Operation("set", "metadata", "error")
			p.IncCacheL2ProcessingError("get", "response", "decode")
			p.IncCacheL2ProcessingError("get", "response", "decode")
			p.IncCacheL2ProcessingError("set", "metadata", "encode")
			after, err := registry.Gather()
			require.NoError(t, err)
			families := make(map[string]*dto.MetricFamily)
			for _, family := range after {
				families[family.GetName()] = family
			}
			for name, family := range legacy {
				require.True(t, proto.Equal(family, families[name]), "legacy metric %s changed", name)
			}

			for _, want := range []struct {
				name    string
				samples map[string]float64
			}{
				{"l2_operations_total", map[string]float64{
					"get/response/success": 2, "set/metadata/error": 1,
				}},
				{"l2_processing_errors_total", map[string]float64{
					"get/response/decode": 2, "set/metadata/encode": 1,
				}},
			} {
				family := families[tc.namespace+"_cache_"+want.name]
				require.NotNil(t, family)
				require.Equal(t, dto.MetricType_COUNTER, family.GetType())
				require.Len(t, family.Metric, len(want.samples))
				for _, sample := range family.Metric {
					labels := map[string]string{}
					for _, label := range sample.Label {
						labels[label.GetName()] = label.GetValue()
					}
					third := "outcome"
					if want.name == "l2_processing_errors_total" {
						third = "stage"
					}
					require.Len(t, labels, 3)
					require.Contains(t, labels, "operation")
					require.Contains(t, labels, "purpose")
					require.Contains(t, labels, third)
					key := labels["operation"] + "/" + labels["purpose"] + "/" + labels[third]
					count, ok := want.samples[key]
					require.True(t, ok, "unexpected labels: %v", labels)
					require.Equal(t, count, sample.GetCounter().GetValue())
					delete(want.samples, key)
				}
				require.Empty(t, want.samples)
			}
			require.Len(t, families, len(legacy)+2)
		})
	}
}
