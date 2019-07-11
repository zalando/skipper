package builtin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/routing"
)

var time0 = time.Now().Truncate(time.Second).UTC()
var time1 = time.Now().Add(1)

func TestRouteCreationMetrics_Do(t *testing.T) {
	for _, tt := range []struct {
		name            string
		route           routing.Route
		expectedMetrics []string
	}{
		{
			name:  "no start time provided",
			route: routing.Route{},
		},
		{
			name:            "start time provided",
			route:           routing.Route{Filters: []*routing.RouteFilter{{Filter: OriginMarker{Origin: "origin", Id: "config1", Created: time0}}}},
			expectedMetrics: []string{"routeCreationTime.origin"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := metricstest.MockMetrics{}
			NewRouteCreationMetrics(&metrics).Do([]*routing.Route{&tt.route})

			metrics.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Len(t, measures, len(tt.expectedMetrics))

				for _, e := range tt.expectedMetrics {
					assert.Containsf(t, measures, e, "measure metrics do not contain %q", e)
				}
			})
		})
	}
}

func TestRouteCreationMetrics_startTimes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		route    routing.Route
		expected map[string]time.Time
	}{
		{
			name:     "no start time provided",
			route:    routing.Route{},
			expected: map[string]time.Time{},
		},
		{
			name: "start time from origin marker",
			route: routing.Route{Filters: []*routing.RouteFilter{
				{Filter: OriginMarker{Origin: "origin", Id: "config0", Created: time0}},
				{Filter: OriginMarker{Origin: "origin", Id: "config1", Created: time1}},
			}},
			expected: map[string]time.Time{"origin": time0},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := metricstest.MockMetrics{}
			s := NewRouteCreationMetrics(&metrics).startTimes(&tt.route)

			assert.Equal(t, tt.expected, s)
		})
	}
}

func TestRouteCreationMetrics_pruneCache(t *testing.T) {
	for _, tt := range []struct {
		name              string
		configIds         map[string]map[string]int
		expectedConfigIds map[string]map[string]int
	}{
		{
			name:              "age increased",
			configIds:         map[string]map[string]int{"origin": {"config0": 0, "config1": 1}},
			expectedConfigIds: map[string]map[string]int{"origin": {"config0": 1, "config1": 2}},
		},
		{
			name:              "entry pruned",
			configIds:         map[string]map[string]int{"origin": {"config0": 0, "config1": maxAge}},
			expectedConfigIds: map[string]map[string]int{"origin": {"config0": 1}},
		},
		{
			name:              "last entry pruned",
			configIds:         map[string]map[string]int{"origin": {"config1": maxAge}},
			expectedConfigIds: map[string]map[string]int{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &RouteCreationMetrics{
				originIdAges: tt.configIds,
			}
			m.pruneCache()
			assert.Equal(t, tt.expectedConfigIds, m.originIdAges)
		})
	}
}

func TestRouteCreationMetrics_filterStartTime(t *testing.T) {
	for _, tt := range []struct {
		name            string
		configIds       map[string]map[string]int
		filter          filters.Filter
		expectedOrigin  string
		expectedCreated time.Time
	}{
		{
			name:            "not config info",
			configIds:       map[string]map[string]int{},
			filter:          &filtertest.Filter{},
			expectedOrigin:  "",
			expectedCreated: time.Time{},
		},
		{
			name:            "config info with no time",
			configIds:       map[string]map[string]int{},
			filter:          OriginMarker{},
			expectedOrigin:  "",
			expectedCreated: time.Time{},
		},
		{
			name:            "no config exists",
			configIds:       map[string]map[string]int{},
			filter:          OriginMarker{Origin: "origin", Id: "config1", Created: time0},
			expectedOrigin:  "origin",
			expectedCreated: time0,
		},
		{
			name:            "same config",
			configIds:       map[string]map[string]int{"origin": {"config0": 0}},
			filter:          OriginMarker{Origin: "origin", Id: "config0", Created: time0},
			expectedOrigin:  "",
			expectedCreated: time.Time{},
		},
		{
			name:            "new config",
			configIds:       map[string]map[string]int{"origin": {"config0": 0}},
			filter:          OriginMarker{Origin: "origin", Id: "config1", Created: time1},
			expectedOrigin:  "origin",
			expectedCreated: time1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &RouteCreationMetrics{
				originIdAges: tt.configIds,
			}
			o, c := m.originStartTime(tt.filter)
			assert.Equal(t, tt.expectedOrigin, o)
			assert.Equal(t, tt.expectedCreated, c)
		})
	}
}
