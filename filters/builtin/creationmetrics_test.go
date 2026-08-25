package builtin

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

var time0 = time.Now().Truncate(time.Second).UTC()
var time1 = time0.Add(time.Second)
var timeNow = time1.Add(time.Second)

func TestRouteCreationMetrics_reportRouteCreationTimes(t *testing.T) {
	f, _ := NewOriginMarkerSpec().CreateFilter([]any{"origin", "config1", time0})
	for _, tt := range []struct {
		name            string
		routes          []*routing.Route
		initialized     bool
		expectedMetrics map[string][]time.Duration
	}{
		{
			name:            "no start time provided",
			routes:          nil,
			initialized:     true,
			expectedMetrics: map[string][]time.Duration{},
		},
		{
			name:            "start time provided",
			routes:          []*routing.Route{{Filters: []*routing.RouteFilter{{Filter: f}}}},
			initialized:     true,
			expectedMetrics: map[string][]time.Duration{"routeCreationTime.origin": {2 * time.Second}},
		},
		{
			name: "first run doesn't provide metrics, just fills the cache (this origin was seen by the previous skipper instance)",
			routes: []*routing.Route{
				{
					Filters: []*routing.RouteFilter{
						{Filter: &OriginMarker{Origin: "origin", Id: "config0", Created: time0}},
					},
				},
				{
					Filters: []*routing.RouteFilter{
						{Filter: &OriginMarker{Origin: "origin", Id: "config1", Created: time0}},
					},
				},
			},
			initialized:     false,
			expectedMetrics: map[string][]time.Duration{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := metricstest.MockMetrics{Now: timeNow}
			creationMetrics := NewRouteCreationMetrics(&metrics)
			creationMetrics.initialized = tt.initialized
			creationMetrics.reportRouteCreationTimes(tt.routes)

			metrics.WithMeasures(func(measures map[string][]time.Duration) {
				assert.Equal(t, tt.expectedMetrics, measures)
			})

			assert.True(t, creationMetrics.initialized)
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
				{Filter: &OriginMarker{Origin: "origin", Id: "config0", Created: time0}},
				{Filter: &OriginMarker{Origin: "origin", Id: "config1", Created: time1}},
			}},
			expected: map[string]time.Time{"origin": time0},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := NewRouteCreationMetrics(&metricstest.MockMetrics{})
			assert.Equal(t, tt.expected, metrics.startTimes(&tt.route))
			//should be cached
			assert.Empty(t, metrics.startTimes(&tt.route))
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

func TestRouteCreationMetrics_originStartTime(t *testing.T) {
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
			filter:          &OriginMarker{},
			expectedOrigin:  "",
			expectedCreated: time.Time{},
		},
		{
			name:            "no config exists",
			configIds:       map[string]map[string]int{},
			filter:          &OriginMarker{Origin: "origin", Id: "config1", Created: time0},
			expectedOrigin:  "origin",
			expectedCreated: time0,
		},
		{
			name:            "same config",
			configIds:       map[string]map[string]int{"origin": {"config0": 0}},
			filter:          &OriginMarker{Origin: "origin", Id: "config0", Created: time0},
			expectedOrigin:  "",
			expectedCreated: time.Time{},
		},
		{
			name:            "new config",
			configIds:       map[string]map[string]int{"origin": {"config0": 0}},
			filter:          &OriginMarker{Origin: "origin", Id: "config1", Created: time1},
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

func TestRouteCreationMetricsMarkerDropped(t *testing.T) {
	for title, routesDoc := range map[string]string{
		"no origin marker": `* -> status(200) -> <shunt>`,

		"origin marker first": `*
			-> originMarker(
				"foo",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> status(200)
			-> <shunt>
		`,

		"origin marker last": `*
			-> status(200)
			-> originMarker(
				"foo",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> <shunt>
		`,

		"origin marker between": `*
			-> status(200)
			-> originMarker(
				"foo",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> inlineContent("bar")
			-> <shunt>
		`,

		"origin markers around": `*
			-> originMarker(
				"foo",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> status(200)
			-> originMarker(
				"bar",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> <shunt>
		`,

		"multiple origin markers between and around": `*
			-> compress()
			-> originMarker(
				"foo",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> originMarker(
				"bar",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> status(200)
			-> originMarker(
				"baz",
				"15d2f2a3-e9ca-11e9-9076-028161d12104",
				"2006-01-02T15:04:05Z"
			)
			-> inlineContent("Hello, world!")
			-> <shunt>
		`,
	} {
		t.Run(title, func(t *testing.T) {
			dc, err := testdataclient.NewDoc(routesDoc)
			if err != nil {
				t.Fatal(err)
			}
			defer dc.Close()

			filtersBefore := make(map[string]bool)
			er, err := dc.LoadAll()
			if err != nil {
				t.Fatal(err)
			}

			for _, r := range er {
				for _, f := range r.Filters {
					filtersBefore[f.Name] = true
				}
			}

			rt := routing.New(routing.Options{
				FilterRegistry:  MakeRegistry(),
				DataClients:     []routing.DataClient{dc},
				PostProcessors:  []routing.PostProcessor{NewRouteCreationMetrics(&metricstest.MockMetrics{})},
				SignalFirstLoad: true,
			})
			defer rt.Close()

			<-rt.FirstLoad()
			r, _ := rt.Route(&http.Request{URL: &url.URL{Path: "/"}})
			if r == nil {
				t.Fatal("failed to find route")
			}

			filtersAfter := make(map[string]bool)
			for _, f := range r.Filters {
				filtersAfter[f.Name] = true
			}

			for name := range filtersBefore {
				if name == "originMarker" {
					if filtersAfter[name] {
						t.Fatal("origin marker was not removed")
					}

					continue
				}

				if !filtersAfter[name] {
					t.Fatal("non-origin marker filters was removed")
				}
			}
		})
	}
}
