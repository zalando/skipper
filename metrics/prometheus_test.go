package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/metrics"
)

func TestPrometheusMetrics(t *testing.T) {

	tests := []struct {
		name       string
		opts       metrics.Options
		addMetrics func(*metrics.Prometheus)
		expMetrics []string
		expCode    int
	}{
		{
			name: "Incrementing the routing failures should get the total of routing failures.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.IncRoutingFailures()
				pm.IncRoutingFailures()
				pm.IncRoutingFailures()
			},
			expMetrics: []string{
				`skipper_route_error_total 3`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Incrementing the backend failures should get the total of backend failures.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.IncErrorsBackend("route1")
				pm.IncErrorsBackend("route2")
				pm.IncErrorsBackend("route1")
			},
			expMetrics: []string{
				`skipper_backend_error_total{route="route1"} 2`,
				`skipper_backend_error_total{route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring the routes lookup should get the duration of the routes lookup.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureRouteLookup(time.Now().Add(-15 * time.Millisecond))
				pm.MeasureRouteLookup(time.Now().Add(-3 * time.Millisecond))
			},
			expMetrics: []string{
				`skipper_route_lookup_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="1"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="5"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="10"} 2`,
				`skipper_route_lookup_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_route_lookup_duration_seconds_sum 0.018`,
				`skipper_route_lookup_duration_seconds_count 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Filter creation latency",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureFilterCreate("filter1", time.Now().Add(-15*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.005"} 0`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.01"} 0`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.025"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.05"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.1"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.25"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="0.5"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="1"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="2.5"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="5"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="10"} 1`,
				`skipper_filter_create_duration_seconds_bucket{filter="filter1",le="+Inf"} 1`,
				`skipper_filter_create_duration_seconds_sum{filter="filter1"} 0.015`,
				`skipper_filter_create_duration_seconds_count{filter="filter1"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring the filter requests should get the duration of the filter requests.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureFilterRequest("filter1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureFilterRequest("filter2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.005"} 0`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.01"} 0`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.025"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.05"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.1"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.25"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="0.5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="1"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="2.5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="10"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter1",le="+Inf"} 1`,
				`skipper_filter_request_duration_seconds_sum{filter="filter1"} 0.015`,
				`skipper_filter_request_duration_seconds_count{filter="filter1"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.005"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.01"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.025"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.05"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.1"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.25"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="0.5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="1"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="2.5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="5"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="10"} 1`,
				`skipper_filter_request_duration_seconds_bucket{filter="filter2",le="+Inf"} 1`,
				`skipper_filter_request_duration_seconds_sum{filter="filter2"} 0.003`,
				`skipper_filter_request_duration_seconds_count{filter="filter2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all filter requests without route specific ones should get the duration of the aggregation of all filter requests only.",
			opts: metrics.Options{EnableAllFiltersMetrics: false},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureAllFiltersRequest("route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureAllFiltersRequest("route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="1"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="10"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_sum 0.018`,
				`skipper_filter_all_combined_request_duration_seconds_count 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all filter requests with route specific ones should get the duration of all filter requests.",
			opts: metrics.Options{EnableAllFiltersMetrics: true},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureAllFiltersRequest("route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureAllFiltersRequest("route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="1"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="5"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="10"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_filter_all_combined_request_duration_seconds_sum 0.018`,
				`skipper_filter_all_combined_request_duration_seconds_count 2`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.005"} 0`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.01"} 0`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.025"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.05"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.1"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.25"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="0.5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="1"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="2.5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="10"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route1",le="+Inf"} 1`,
				`skipper_filter_all_request_duration_seconds_sum{route="route1"} 0.015`,
				`skipper_filter_all_request_duration_seconds_count{route="route1"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.005"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.01"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.025"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.05"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.1"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.25"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="0.5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="1"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="2.5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="5"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="10"} 1`,
				`skipper_filter_all_request_duration_seconds_bucket{route="route2",le="+Inf"} 1`,
				`skipper_filter_all_request_duration_seconds_sum{route="route2"} 0.003`,
				`skipper_filter_all_request_duration_seconds_count{route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all backend latency without route specific ones should get the duration of the aggregation of all backend latency only.",
			opts: metrics.Options{EnableRouteBackendMetrics: false},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureBackend("route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureBackend("route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_backend_combined_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="1"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="10"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_backend_combined_duration_seconds_sum 0.018`,
				`skipper_backend_combined_duration_seconds_count 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all backend latency with route specific ones should get the duration of all backend latency.",
			opts: metrics.Options{EnableRouteBackendMetrics: true},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureBackend("route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureBackend("route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_backend_combined_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="1"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="5"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="10"} 2`,
				`skipper_backend_combined_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_backend_combined_duration_seconds_sum 0.018`,
				`skipper_backend_combined_duration_seconds_count 2`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.005"} 0`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.01"} 0`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.025"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.05"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.25"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="0.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="2.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="10"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route1",le="+Inf"} 1`,
				`skipper_backend_duration_seconds_sum{host="",route="route1"} 0.015`,
				`skipper_backend_duration_seconds_count{host="",route="route1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.005"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.01"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.025"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.05"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.25"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="0.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="2.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="10"} 1`,
				`skipper_backend_duration_seconds_bucket{host="",route="route2",le="+Inf"} 1`,
				`skipper_backend_duration_seconds_sum{host="",route="route2"} 0.003`,
				`skipper_backend_duration_seconds_count{host="",route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all backend latency without host specific ones should get nothing.",
			opts: metrics.Options{EnableBackendHostMetrics: false},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureBackendHost("host1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureBackendHost("host2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{},
			expCode:    http.StatusOK,
		},
		{
			name: "Measuring all backend latency with host specific ones should get the duration of the aggregation of all backend host latency.",
			opts: metrics.Options{EnableBackendHostMetrics: true},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureBackendHost("host1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureBackendHost("host2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.005"} 0`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.01"} 0`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.025"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.05"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.25"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="0.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="2.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="10"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host1",route="",le="+Inf"} 1`,
				`skipper_backend_duration_seconds_sum{host="host1",route=""} 0.015`,
				`skipper_backend_duration_seconds_count{host="host1",route=""} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.005"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.01"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.025"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.05"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.25"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="0.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="1"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="2.5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="5"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="10"} 1`,
				`skipper_backend_duration_seconds_bucket{host="host2",route="",le="+Inf"} 1`,
				`skipper_backend_duration_seconds_sum{host="host2",route=""} 0.003`,
				`skipper_backend_duration_seconds_count{host="host2",route=""} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring filter response duration should get filter response latency.",
			opts: metrics.Options{EnableBackendHostMetrics: false},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureFilterResponse("filter1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureFilterResponse("filter2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.005"} 0`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.01"} 0`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.025"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.05"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.1"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.25"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="0.5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="1"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="2.5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="10"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter1",le="+Inf"} 1`,
				`skipper_filter_response_duration_seconds_sum{filter="filter1"} 0.015`,
				`skipper_filter_response_duration_seconds_count{filter="filter1"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.005"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.01"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.025"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.05"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.1"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.25"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="0.5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="1"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="2.5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="5"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="10"} 1`,
				`skipper_filter_response_duration_seconds_bucket{filter="filter2",le="+Inf"} 1`,
				`skipper_filter_response_duration_seconds_sum{filter="filter2"} 0.003`,
				`skipper_filter_response_duration_seconds_count{filter="filter2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring filter response duration without filter specific ones only should get combined filter response latency.",
			opts: metrics.Options{EnableAllFiltersMetrics: false},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureAllFiltersResponse("filter1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureAllFiltersResponse("filter2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="1"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="10"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_sum 0.018`,
				`skipper_filter_all_combined_response_duration_seconds_count 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring filter response duration without filter specific ones only should get all filter response latency.",
			opts: metrics.Options{EnableAllFiltersMetrics: true},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureAllFiltersResponse("filter1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureAllFiltersResponse("filter2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="1"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="5"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="10"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_filter_all_combined_response_duration_seconds_sum 0.018`,
				`skipper_filter_all_combined_response_duration_seconds_count 2`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.005"} 0`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.01"} 0`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.025"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.05"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.1"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.25"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="0.5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="1"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="2.5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="10"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter1",le="+Inf"} 1`,
				`skipper_filter_all_response_duration_seconds_sum{route="filter1"} 0.015`,
				`skipper_filter_all_response_duration_seconds_count{route="filter1"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.005"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.01"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.025"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.05"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.1"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.25"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="0.5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="1"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="2.5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="5"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="10"} 1`,
				`skipper_filter_all_response_duration_seconds_bucket{route="filter2",le="+Inf"} 1`,
				`skipper_filter_all_response_duration_seconds_sum{route="filter2"} 0.003`,
				`skipper_filter_all_response_duration_seconds_count{route="filter2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring only combined response, should measure responses latency without route.",
			opts: metrics.Options{
				EnableCombinedResponseMetrics: true,
				EnableRouteResponseMetrics:    false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureResponse(301, "GET", "route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureResponse(200, "POST", "route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.005"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.01"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.025"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.05"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.1"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.25"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="0.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="1"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="2.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="10"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="",le="+Inf"} 1`,
				`skipper_response_duration_seconds_sum{code="200",method="POST",route=""} 0.003`,
				`skipper_response_duration_seconds_count{code="200",method="POST",route=""} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.005"} 0`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.01"} 0`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.025"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.05"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.1"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.25"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="0.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="1"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="2.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="10"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="",le="+Inf"} 1`,
				`skipper_response_duration_seconds_sum{code="301",method="GET",route=""} 0.015`,
				`skipper_response_duration_seconds_count{code="301",method="GET",route=""} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all responses, should measure responses latency.",
			opts: metrics.Options{
				EnableCombinedResponseMetrics: false,
				EnableRouteResponseMetrics:    true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureResponse(301, "GET", "route1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureResponse(200, "POST", "route2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.005"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.01"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.025"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.05"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.1"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.25"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="1"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="2.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="5"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="10"} 1`,
				`skipper_response_duration_seconds_bucket{code="200",method="POST",route="route2",le="+Inf"} 1`,
				`skipper_response_duration_seconds_sum{code="200",method="POST",route="route2"} 0.003`,
				`skipper_response_duration_seconds_count{code="200",method="POST",route="route2"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.005"} 0`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.01"} 0`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.025"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.05"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.1"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.25"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="1"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="2.5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="5"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="10"} 1`,
				`skipper_response_duration_seconds_bucket{code="301",method="GET",route="route1",le="+Inf"} 1`,
				`skipper_response_duration_seconds_sum{code="301",method="GET",route="route1"} 0.015`,
				`skipper_response_duration_seconds_count{code="301",method="GET",route="route1"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by route, only should measure served latency by route.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     true,
				EnableServeRouteCounter:     true,
				EnableServeHostMetrics:      false,
				EnableServeHostCounter:      false,
				EnableServeMethodMetric:     true,
				EnableServeStatusCodeMetric: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.005"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.01"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",method="POST",route="route2",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{code="200",method="POST",route="route2"} 0.003`,
				`skipper_serve_route_duration_seconds_count{code="200",method="POST",route="route2"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.005"} 0`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.01"} 0`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",method="GET",route="route1",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{code="301",method="GET",route="route1"} 0.015`,
				`skipper_serve_route_duration_seconds_count{code="301",method="GET",route="route1"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by route, only should measure served latency by route without method.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     true,
				EnableServeRouteCounter:     true,
				EnableServeHostMetrics:      false,
				EnableServeHostCounter:      false,
				EnableServeMethodMetric:     false,
				EnableServeStatusCodeMetric: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.005"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.01"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="200",route="route2",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{code="200",route="route2"} 0.003`,
				`skipper_serve_route_duration_seconds_count{code="200",route="route2"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.005"} 0`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.01"} 0`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{code="301",route="route1",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{code="301",route="route1"} 0.015`,
				`skipper_serve_route_duration_seconds_count{code="301",route="route1"} 1`,
				`skipper_serve_route_count{code="301",method="GET",route="route1"} 1`,
				`skipper_serve_route_count{code="200",method="POST",route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by route, only should measure served latency by route without code.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     true,
				EnableServeRouteCounter:     true,
				EnableServeHostMetrics:      false,
				EnableServeHostCounter:      false,
				EnableServeMethodMetric:     true,
				EnableServeStatusCodeMetric: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.005"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.01"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="POST",route="route2",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{method="POST",route="route2"} 0.003`,
				`skipper_serve_route_duration_seconds_count{method="POST",route="route2"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.005"} 0`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.01"} 0`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{method="GET",route="route1",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{method="GET",route="route1"} 0.015`,
				`skipper_serve_route_duration_seconds_count{method="GET",route="route1"} 1`,
				`skipper_serve_route_count{code="301",method="GET",route="route1"} 1`,
				`skipper_serve_route_count{code="200",method="POST",route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by route, only should measure served latency by route without method and code.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     true,
				EnableServeRouteCounter:     true,
				EnableServeHostMetrics:      false,
				EnableServeHostCounter:      false,
				EnableServeMethodMetric:     false,
				EnableServeStatusCodeMetric: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.005"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.01"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route2",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{route="route2"} 0.003`,
				`skipper_serve_route_duration_seconds_count{route="route2"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.005"} 0`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.01"} 0`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.025"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.05"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.25"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="0.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="1"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="2.5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="5"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="10"} 1`,
				`skipper_serve_route_duration_seconds_bucket{route="route1",le="+Inf"} 1`,
				`skipper_serve_route_duration_seconds_sum{route="route1"} 0.015`,
				`skipper_serve_route_duration_seconds_count{route="route1"} 1`,
				`skipper_serve_route_count{code="301",method="GET",route="route1"} 1`,
				`skipper_serve_route_count{code="200",method="POST",route="route2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by route, should measure served latency by host.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     false,
				EnableServeRouteCounter:     false,
				EnableServeHostMetrics:      true,
				EnableServeHostCounter:      true,
				EnableServeMethodMetric:     true,
				EnableServeStatusCodeMetric: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.005"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.01"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",method="POST",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{code="200",host="host2",method="POST"} 0.003`,
				`skipper_serve_host_duration_seconds_count{code="200",host="host2",method="POST"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.005"} 0`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.01"} 0`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",method="GET",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{code="301",host="host1",method="GET"} 0.015`,
				`skipper_serve_host_duration_seconds_count{code="301",host="host1",method="GET"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by hosts, only should measure served latency by route without method.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     false,
				EnableServeRouteCounter:     false,
				EnableServeHostMetrics:      true,
				EnableServeHostCounter:      true,
				EnableServeMethodMetric:     false,
				EnableServeStatusCodeMetric: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.005"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.01"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="200",host="host2",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{code="200",host="host2"} 0.003`,
				`skipper_serve_host_duration_seconds_count{code="200",host="host2"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.005"} 0`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.01"} 0`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{code="301",host="host1",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{code="301",host="host1"} 0.015`,
				`skipper_serve_host_duration_seconds_count{code="301",host="host1"} 1`,
				`skipper_serve_host_count{code="301",host="host1",method="GET"} 1`,
				`skipper_serve_host_count{code="200",host="host2",method="POST"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by hosts, only should measure served latency by route without code.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     false,
				EnableServeRouteCounter:     false,
				EnableServeHostMetrics:      true,
				EnableServeHostCounter:      true,
				EnableServeMethodMetric:     true,
				EnableServeStatusCodeMetric: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.005"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.01"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",method="POST",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{host="host2",method="POST"} 0.003`,
				`skipper_serve_host_duration_seconds_count{host="host2",method="POST"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.005"} 0`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.01"} 0`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",method="GET",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{host="host1",method="GET"} 0.015`,
				`skipper_serve_host_duration_seconds_count{host="host1",method="GET"} 1`,
				`skipper_serve_host_count{code="301",host="host1",method="GET"} 1`,
				`skipper_serve_host_count{code="200",host="host2",method="POST"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts split by hosts, only should measure served latency by route without method and code.",
			opts: metrics.Options{
				EnableServeRouteMetrics:     false,
				EnableServeRouteCounter:     false,
				EnableServeHostMetrics:      true,
				EnableServeHostCounter:      true,
				EnableServeMethodMetric:     false,
				EnableServeStatusCodeMetric: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.005"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.01"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host2",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{host="host2"} 0.003`,
				`skipper_serve_host_duration_seconds_count{host="host2"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.005"} 0`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.01"} 0`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.025"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.05"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.25"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="0.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="1"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="2.5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="5"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="10"} 1`,
				`skipper_serve_host_duration_seconds_bucket{host="host1",le="+Inf"} 1`,
				`skipper_serve_host_duration_seconds_sum{host="host1"} 0.015`,
				`skipper_serve_host_duration_seconds_count{host="host1"} 1`,
				`skipper_serve_host_count{code="301",host="host1",method="GET"} 1`,
				`skipper_serve_host_count{code="200",host="host2",method="POST"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all serves by the hosts and route, only counters.",
			opts: metrics.Options{
				EnableServeRouteMetrics: false,
				EnableServeRouteCounter: true,
				EnableServeHostMetrics:  false,
				EnableServeHostCounter:  true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureServe("route1", "host1", "GET", 301, time.Now().Add(-15*time.Millisecond))
				pm.MeasureServe("route2", "host2", "POST", 200, time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_serve_host_count{code="301",host="host1",method="GET"} 1`,
				`skipper_serve_host_count{code="200",host="host2",method="POST"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Incrementing the backend streaming errors should get the total of backend 5xx errors.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.IncErrorsStreaming("route1")
				pm.IncErrorsStreaming("route2")
				pm.IncErrorsStreaming("route1")
				pm.IncErrorsStreaming("route3")
			},
			expMetrics: []string{
				`skipper_streaming_error_total{route="route1"} 2`,
				`skipper_streaming_error_total{route="route2"} 1`,
				`skipper_streaming_error_total{route="route3"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring all backend 5xx, should measure backend 5xx latency.",
			opts: metrics.Options{},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureBackend5xx(time.Now().Add(-15 * time.Millisecond))
				pm.MeasureBackend5xx(time.Now().Add(-3 * time.Millisecond))
			},
			expMetrics: []string{
				`skipper_backend_5xx_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.025"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.05"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.1"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.25"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="0.5"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="1"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="2.5"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="5"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="10"} 2`,
				`skipper_backend_5xx_duration_seconds_bucket{le="+Inf"} 2`,
				`skipper_backend_5xx_duration_seconds_sum 0.018`,
				`skipper_backend_5xx_duration_seconds_count 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Measuring custom metrics, should measure custom metrics latency.",
			opts: metrics.Options{},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureSince("key1", time.Now().Add(-15*time.Millisecond))
				pm.MeasureSince("key2", time.Now().Add(-3*time.Millisecond))
			},
			expMetrics: []string{
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.005"} 0`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.01"} 0`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.025"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.05"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.1"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.25"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="0.5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="1"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="2.5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="10"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key1",le="+Inf"} 1`,
				`skipper_custom_duration_seconds_sum{key="key1"} 0.015`,
				`skipper_custom_duration_seconds_count{key="key1"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.005"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.01"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.025"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.05"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.1"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.25"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="0.5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="1"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="2.5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="5"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="10"} 1`,
				`skipper_custom_duration_seconds_bucket{key="key2",le="+Inf"} 1`,
				`skipper_custom_duration_seconds_sum{key="key2"} 0.003`,
				`skipper_custom_duration_seconds_count{key="key2"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Incrementing the custom metric counter should get the total custom metrics.",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.IncCounter("key1")
				pm.IncCounter("key2")
				pm.IncCounter("key1")
				pm.IncCounter("key3")
			},
			expMetrics: []string{
				`skipper_custom_total{key="key1"} 2`,
				`skipper_custom_total{key="key2"} 1`,
				`skipper_custom_total{key="key3"} 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Updating custom gauges should update custom gauges in the gauges custom metrics",
			addMetrics: func(pm *metrics.Prometheus) {
				pm.UpdateGauge("key1", 1)
				pm.UpdateGauge("key2", 2)
				pm.UpdateGauge("key1", 3)
			},
			expMetrics: []string{
				`skipper_custom_gauges{key="key1"} 3`,
				`skipper_custom_gauges{key="key2"} 2`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Skipper latency metrics should be registered even when request and response are disabled",
			opts: metrics.Options{
				EnableProxyRequestMetrics:  false,
				EnableProxyResponseMetrics: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureProxy(5*time.Millisecond, 10*time.Millisecond)
			},
			expMetrics: []string{
				`skipper_proxy_total_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.01"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_total_duration_seconds_sum 0.015`,
				`skipper_proxy_total_duration_seconds_count 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Skipper latency metrics, request enabled and response are disabled",
			opts: metrics.Options{
				EnableProxyRequestMetrics:  true,
				EnableProxyResponseMetrics: false,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureProxy(5*time.Millisecond, 10*time.Millisecond)
			},
			expMetrics: []string{
				`skipper_proxy_total_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.01"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_total_duration_seconds_sum 0.015`,
				`skipper_proxy_total_duration_seconds_count 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_request_duration_seconds_sum 0.005`,
				`skipper_proxy_request_duration_seconds_count 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Skipper latency metrics, request and response are enabled",
			opts: metrics.Options{
				EnableProxyRequestMetrics:  true,
				EnableProxyResponseMetrics: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureProxy(5*time.Millisecond, 10*time.Millisecond)
			},
			expMetrics: []string{
				`skipper_proxy_total_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.01"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_total_duration_seconds_sum 0.015`,
				`skipper_proxy_total_duration_seconds_count 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.005"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_request_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_request_duration_seconds_sum 0.005`,
				`skipper_proxy_request_duration_seconds_count 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_response_duration_seconds_sum 0.01`,
				`skipper_proxy_response_duration_seconds_count 1`,
			},
			expCode: http.StatusOK,
		},
		{
			name: "Skipper latency metrics, request disabled and response enabled",
			opts: metrics.Options{
				EnableProxyRequestMetrics:  false,
				EnableProxyResponseMetrics: true,
			},
			addMetrics: func(pm *metrics.Prometheus) {
				pm.MeasureProxy(5*time.Millisecond, 10*time.Millisecond)
			},
			expMetrics: []string{
				`skipper_proxy_total_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.01"} 0`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_total_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_total_duration_seconds_sum 0.015`,
				`skipper_proxy_total_duration_seconds_count 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.005"} 0`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.01"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.025"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.05"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.1"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.25"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="0.5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="1"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="2.5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="5"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="10"} 1`,
				`skipper_proxy_response_duration_seconds_bucket{le="+Inf"} 1`,
				`skipper_proxy_response_duration_seconds_sum 0.01`,
				`skipper_proxy_response_duration_seconds_count 1`,
			},
			expCode: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pm := metrics.NewPrometheus(test.opts)
			path := "/awesome-metrics"

			// Create the muxer and register as handler on the Metrics service.
			mux := http.NewServeMux()
			pm.RegisterHandler(path, mux)

			// Add the required metrics.
			test.addMetrics(pm)

			// Make the request to the metrics.
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// Check.
			resp := w.Result()
			if test.expCode != resp.StatusCode {
				t.Errorf("metrics service returned an incorrect status code, should be: %d, got: %d", test.expCode, resp.StatusCode)
			} else {
				body, _ := io.ReadAll(resp.Body)
				// Check all the metrics are present.
				for _, expMetric := range test.expMetrics {
					if ok := strings.Contains(string(body), expMetric); !ok {
						t.Errorf("'%s' metric not present on the result of metrics service", expMetric)
					}
				}
			}
		})
	}
}

func TestPrometheusMetricsStartTimestamp(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{
		EnablePrometheusStartLabel: true,
		EnableServeHostCounter:     true,
	})
	path := "/awesome-metrics"

	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	before := time.Now()

	pm.MeasureServe("route1", "foo.test", "GET", 200, time.Now().Add(-15*time.Millisecond))
	pm.MeasureServe("route1", "bar.test", "POST", 201, time.Now().Add(-15*time.Millisecond))
	pm.MeasureServe("route1", "bar.test", "POST", 201, time.Now().Add(-15*time.Millisecond))
	pm.IncRoutingFailures()
	pm.IncRoutingFailures()
	pm.IncRoutingFailures()

	after := time.Now()

	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	t.Logf("Metrics response:\n%s", body)

	// Prometheus client does not allow to mock counter creation timestamps,
	// see https://github.com/prometheus/client_golang/issues/1354
	//
	// checkMetric tests that timestamp is within [before, after] range.
	checkMetric := func(pattern string) {
		t.Helper()

		re := regexp.MustCompile(pattern)

		matches := re.FindSubmatch(body)
		require.NotNil(t, matches, "Metrics response does not match: %s", pattern)
		require.Len(t, matches, 2)

		ts, err := strconv.ParseInt(string(matches[1]), 10, 64)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, ts, before.UnixNano())
		assert.LessOrEqual(t, ts, after.UnixNano())
	}

	checkMetric(`skipper_serve_host_count{code="200",host="foo_test",method="GET",start="(\d+)"} 1`)
	checkMetric(`skipper_serve_host_count{code="201",host="bar_test",method="POST",start="(\d+)"} 2`)
	checkMetric(`skipper_route_error_total{start="(\d+)"} 3`)
}
