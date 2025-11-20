package routing_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestNoMultipleTreePredicates(t *testing.T) {
	for _, ti := range []struct {
		routes string
		err    bool
	}{{
		`Path("/foo") && Path("/bar") -> <shunt>`,
		true,
	}, {
		`Path("/foo") && PathSubtree("/bar") -> <shunt>`,
		true,
	}, {
		`PathSubtree("/foo") && PathSubtree("/bar") -> <shunt>`,
		true,
	}, {
		`Path("/foo") -> <shunt>`,
		false,
	}, {
		`PathSubtree("/foo") -> <shunt>`,
		false,
	}} {
		func() {
			dc, err := testdataclient.NewDoc(ti.routes)
			if err != nil {
				if !ti.err {
					t.Error(ti.routes, err)
				}

				return
			}
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				if !ti.err {
					t.Error(ti.routes, err)
				}

				return
			}

			erred := false
			o := &routing.Options{
				FilterRegistry: make(filters.Registry),
			}
			pr := make(map[string]routing.PredicateSpec)
			for _, d := range defs {
				if _, err := routing.ExportProcessRouteDef(o, pr, d); err != nil {
					erred = true
					break
				}
			}

			if erred != ti.err {
				t.Error("unexpected error result", erred, ti.err)
			}
		}()
	}
}

func TestProcessRouteDefErrors(t *testing.T) {
	for _, ti := range []struct {
		routes string
		err    string
	}{
		{
			`* -> True() -> <shunt>`,
			`unknown_filter: trying to use "True" as filter, but it is only available as predicate`,
		}, {
			`* -> PathRegexp("/test") -> <shunt>`,
			`unknown_filter: trying to use "PathRegexp" as filter, but it is only available as predicate`,
		}, {
			`* -> Unknown("/test") -> <shunt>`,
			`unknown_filter: filter "Unknown" not found`,
		}, {
			`Unknown()  ->  <shunt>`,
			`unknown_predicate: predicate "Unknown" not found`,
		}, {
			`QueryParam() -> <shunt>`,
			`invalid_predicate_params: failed to create predicate "QueryParam": invalid predicate parameters`,
		}, {
			`* -> setPath() -> <shunt>`,
			`invalid_filter_params: failed to create filter "setPath": invalid filter parameters`,
		},
	} {
		func() {
			dc, err := testdataclient.NewDoc(ti.routes)
			if err != nil {
				t.Error(ti.routes, err)

				return
			}
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				t.Error(ti.routes, err)

				return
			}

			pr := map[string]routing.PredicateSpec{}
			for _, s := range []routing.PredicateSpec{primitive.NewTrue(), query.New()} {
				pr[s.Name()] = s
			}
			fr := make(filters.Registry)
			fr.Register(builtin.NewSetPath())
			o := &routing.Options{
				FilterRegistry: fr,
			}
			for _, d := range defs {
				_, err := routing.ExportProcessRouteDef(o, pr, d)
				if err == nil || err.Error() != ti.err {
					t.Errorf("expected error '%s'. Got: '%s'", ti.err, err)
				}
			}
		}()
	}
}

func TestProcessRouteDefWeight(t *testing.T) {
	cpm := map[string]routing.PredicateSpec{
		"WeightedPredicate10":      weightedPredicateSpec{name: "WeightedPredicate10", weight: 10},
		"WeightedPredicateMinus10": weightedPredicateSpec{name: "WeightedPredicateMinus10", weight: -10},
	}

	for _, ti := range []struct {
		route  string
		weight int
	}{
		{
			`Path("/foo") -> <shunt>`,
			0,
		}, {
			`WeightedPredicate10() -> <shunt>`,
			10,
		}, {
			`Weight(20) -> <shunt>`,
			20,
		}, {
			`Weight(20) && Weight(10)-> <shunt>`,
			30,
		}, {
			`WeightedPredicate10() && Weight(20) -> <shunt>`,
			30,
		}, {
			`WeightedPredicateMinus10() -> <shunt>`,
			-10,
		}, {
			`WeightedPredicateMinus10() && Weight(10) -> <shunt>`,
			0,
		}, {
			`WeightedPredicateMinus10() && Weight(20) -> <shunt>`,
			10,
		},
	} {
		func() {

			dc, err := testdataclient.NewDoc(ti.route)
			if err != nil {
				t.Error(ti.route, err)

				return
			}
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				t.Error(ti.route, err)

				return
			}

			r := defs[0]

			_, weight, err := routing.ExportProcessPredicates(&routing.Options{}, cpm, r.Predicates)
			if err != nil {
				t.Error(ti.route, err)

				return
			}

			if weight != ti.weight {
				t.Errorf("expected weight '%d'. Got: '%d' (%s)", ti.weight, weight, ti.route)

				return
			}
		}()
	}
}

func TestLogging(t *testing.T) {
	const routes = `
		r1_1: Path("/foo") -> "https://foo.example.org";
		r1_2: Path("/bar") -> "https://bar.example.org";
		r1_3: Path("/baz") -> "https://baz.example.org";
		r1_4: Path("/qux") -> "https://qux.example.org";
		r1_5: Path("/quux") -> "https://quux.example.org";
	`

	init := func(l logging.Logger, client routing.DataClient, suppress bool) *routing.Routing {
		return routing.New(routing.Options{
			DataClients:  []routing.DataClient{client},
			Log:          l,
			SuppressLogs: suppress,
		})
	}

	testUpdate := func(
		t *testing.T, suppress bool,
		initQuery string, initCount int,
		upsertQuery string, upsertCount int,
		deleteQuery string, deleteCount int,
	) {
		client, err := testdataclient.NewDoc(routes)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()

		testLog := loggingtest.New()
		defer testLog.Close()

		rt := init(testLog, client, suppress)
		defer rt.Close()

		if err := testLog.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
			t.Error(err)
			return
		}

		count := testLog.Count(initQuery)
		if count != initCount {
			t.Error("unexpected count of log entries", count)
			t.Log("expected", initCount, initQuery)
			t.Log("got     ", count)
			return
		}

		testLog.Reset()

		client.UpdateDoc(
			`r1_1: Path("/foo_mod") -> "https://foo.example.org";
			r1_4: Path("/qux_mod") -> "https://qux.example.org"`,
			[]string{"r1_2"},
		)

		if err := testLog.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
			t.Error(err)
			return
		}

		count = testLog.Count(upsertQuery)
		if count != upsertCount {
			t.Error("unexpected count of log entries", count)
			return
		}

		count = testLog.Count(deleteQuery)
		if count != deleteCount {
			t.Error("unexpected count of log entries", count)
			return
		}
	}

	t.Run("full", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			testUpdate(
				t, false,
				"route settings, reset", 5,
				"route settings, update, route:", 2,
				"route settings, update, deleted", 1,
			)
		})
	})

	t.Run("suppressed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			testUpdate(
				t, true,
				"route settings, reset", 2,
				"route settings, update, upsert count:", 1,
				"route settings, update, delete count:", 1,
			)
		})
	})
}

func TestMetrics(t *testing.T) {
	t.Run("create filter latency", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			client, err := testdataclient.NewDoc(`
			r0: * -> slowCreate("100ms") -> slowCreate("200ms") -> slowCreate("100ms") -> <shunt>;
		`)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			metrics := &metricstest.MockMetrics{
				Now: time.Now(),
			}
			fr := make(filters.Registry)
			fr.Register(slowCreateSpec{})

			r := routing.New(routing.Options{
				DataClients:     []routing.DataClient{client},
				FilterRegistry:  fr,
				Metrics:         metrics,
				SignalFirstLoad: true,
			})
			defer r.Close()
			<-r.FirstLoad()

			metrics.WithMeasures(func(m map[string][]time.Duration) {
				assert.InEpsilonSlice(t, []time.Duration{
					100 * time.Millisecond,
					200 * time.Millisecond,
					100 * time.Millisecond,
				}, m["filter.slowCreate.create"], 0.1)
			})
		})
	})
}

func TestRouteValidationReasonMetrics(t *testing.T) {
	testCases := []struct {
		name                  string
		routes                string
		expectedValid         int
		expectedInvalidRoutes map[string]string // routeId -> reason
	}{
		{
			name: "various error types",
			routes: `
				validRoute: Path("/valid") -> "https://example.org";
				invalidBackend1: Path("/bad1") -> "invalid-url";
				unknownFilter1: Path("/uf1") -> unknownFilter() -> "https://example.org";
				invalidParams1: Path("/ip1") -> setPath() -> "https://example.org";
				invalidParams2: Path("/ip2") -> setPath("too", "many", "params") -> "https://example.org";
			`,
			expectedValid: 1,
			expectedInvalidRoutes: map[string]string{
				"invalidBackend1": "failed_backend_split",
				"unknownFilter1":  "unknown_filter",
				"invalidParams1":  "invalid_filter_params",
				"invalidParams2":  "invalid_filter_params",
			},
		},
		{
			name: "only valid routes",
			routes: `
				validRoute1: Path("/valid1") -> "https://example.org";
				validRoute2: Path("/valid2") -> "https://example.org";
			`,
			expectedValid:         2,
			expectedInvalidRoutes: map[string]string{},
		},
		{
			name: "only invalid backend routes",
			routes: `
				invalidBackend1: Path("/bad1") -> "invalid-url";
			`,
			expectedValid: 0,
			expectedInvalidRoutes: map[string]string{
				"invalidBackend1": "failed_backend_split",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("MockMetrics", func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					testRouteValidationReasonMetricsWithMock(t, tc.routes, tc.expectedValid, tc.expectedInvalidRoutes)
				})
			})

			t.Run("Prometheus", func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					testRouteValidationReasonMetricsWithPrometheus(t, tc.routes, tc.expectedValid, tc.expectedInvalidRoutes)
				})
			})
		})
	}
}

func testRouteValidationReasonMetricsWithMock(t *testing.T, routes string, expectedValid int, expectedInvalidRoutes map[string]string) {
	metrics := &metricstest.MockMetrics{}

	dc, err := testdataclient.NewDoc(routes)
	if err != nil {
		t.Fatal(err)
	}
	fr := make(filters.Registry)
	fr.Register(builtin.NewSetPath())

	r := routing.New(routing.Options{
		DataClients:     []routing.DataClient{dc},
		FilterRegistry:  fr,
		Predicates:      []routing.PredicateSpec{primitive.NewTrue()},
		Metrics:         metrics,
		SignalFirstLoad: true,
	})
	defer func() {
		r.Close()
		dc.Close()
		time.Sleep(100 * time.Millisecond) // Allow goroutines to clean up
	}()
	<-r.FirstLoad()

	waitForIndividualRouteMetrics(t, metrics, int64(expectedValid), expectedInvalidRoutes)
}

func testRouteValidationReasonMetricsWithPrometheus(t *testing.T, routes string, expectedValid int, expectedInvalidRoutes map[string]string) {
	pm := metrics.NewPrometheus(metrics.Options{})
	path := "/metrics"

	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	dc, err := testdataclient.NewDoc(routes)
	if err != nil {
		t.Fatal(err)
	}
	fr := make(filters.Registry)
	fr.Register(builtin.NewSetPath())

	r := routing.New(routing.Options{
		DataClients:     []routing.DataClient{dc},
		FilterRegistry:  fr,
		Predicates:      []routing.PredicateSpec{primitive.NewTrue()},
		Metrics:         pm,
		SignalFirstLoad: true,
	})
	defer func() {
		r.Close()
		dc.Close()
		time.Sleep(100 * time.Millisecond) // Allow goroutines to clean up
	}()
	<-r.FirstLoad()

	// Wait for metrics to be updated
	timeout := time.After(100 * time.Millisecond)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	var output string
	for {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		output = string(body)

		expectedRoutesTotalLine := fmt.Sprintf("skipper_custom_gauges{key=\"routes.total\"} %d", expectedValid)
		if !strings.Contains(output, expectedRoutesTotalLine) {
			select {
			case <-timeout:
				t.Errorf("Expected to find %q in metrics output", expectedRoutesTotalLine)
				t.Logf("Metrics output:\n%s", output)
				return
			case <-ticker.C:
				continue
			}
		}

		// Check individual invalid route metrics
		allFound := true
		for routeId, expectedReason := range expectedInvalidRoutes {
			expectedMetricLine := fmt.Sprintf("skipper_route_invalid{reason=\"%s\",route_id=\"%s\"} 1", expectedReason, routeId)
			if !strings.Contains(output, expectedMetricLine) {
				allFound = false
				break
			}
		}

		if allFound {
			break
		}

		select {
		case <-timeout:
			for routeId, expectedReason := range expectedInvalidRoutes {
				expectedMetricLine := fmt.Sprintf("skipper_route_invalid{reason=\"%s\",route_id=\"%s\"} 1", expectedReason, routeId)
				if !strings.Contains(output, expectedMetricLine) {
					t.Errorf("Expected to find %q in metrics output", expectedMetricLine)
				}
			}
			t.Logf("Metrics output:\n%s", output)
			return
		case <-ticker.C:
			continue
		}
	}
}

func waitForIndividualRouteMetrics(t *testing.T, metrics *metricstest.MockMetrics, expectedValid int64, expectedInvalidRoutes map[string]string) {
	timeout := time.After(100 * time.Millisecond)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		metricsMatch := false
		metrics.WithGauges(func(gauges map[string]float64) {
			if gauges["routes.total"] != float64(expectedValid) {
				return
			}

			// Check that all expected invalid routes have their metrics set to 1
			for routeId, expectedReason := range expectedInvalidRoutes {
				expectedKey := fmt.Sprintf("route.invalid.%s..%s", routeId, expectedReason)
				if gauges[expectedKey] != 1 {
					return
				}
			}

			// Check that we don't have unexpected invalid route metrics
			for key := range gauges {
				if strings.HasPrefix(key, "route.invalid.") && key != "routes.total" {
					// Extract route ID from the key
					parts := strings.Split(key, ".")
					if len(parts) >= 4 {
						routeId := parts[2]
						if _, expected := expectedInvalidRoutes[routeId]; !expected {
							// Allow 0 values (set when routes become valid)
							if gauges[key] != 0 {
								return // Found unexpected invalid route metric
							}
						}
					}
				}
			}

			metricsMatch = true
		})

		if metricsMatch {
			break
		}

		select {
		case <-timeout:
			metrics.WithGauges(func(gauges map[string]float64) {
				if gauges["routes.total"] != float64(expectedValid) {
					t.Errorf("Expected routes.total to be %d, got %f", expectedValid, gauges["routes.total"])
				}

				for routeId, expectedReason := range expectedInvalidRoutes {
					expectedKey := fmt.Sprintf("route.invalid.%s..%s", routeId, expectedReason)
					if gauges[expectedKey] != 1 {
						t.Errorf("Expected metric for invalid route %s with reason %s to be 1, got %f", routeId, expectedReason, gauges[expectedKey])
					}
				}
			})
			return
		case <-ticker.C:
			continue
		}
	}
}

func TestRouteMetricsSetToZeroWhenFixed(t *testing.T) {
	t.Run("MockMetrics", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			testRouteMetricsSetToZeroWhenFixedWithMock(t)
		})
	})

	t.Run("Prometheus", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			testRouteMetricsSetToZeroWhenFixedWithPrometheus(t)
		})
	})
}

func testRouteMetricsSetToZeroWhenFixedWithMock(t *testing.T) {
	metrics := &metricstest.MockMetrics{}

	// Start with an invalid route
	dc, err := testdataclient.NewDoc(`invalidBackend1: Path("/bad1") -> "invalid-url";`)
	if err != nil {
		t.Fatal(err)
	}

	fr := make(filters.Registry)
	fr.Register(builtin.NewSetPath())

	r := routing.New(routing.Options{
		DataClients:     []routing.DataClient{dc},
		FilterRegistry:  fr,
		Predicates:      []routing.PredicateSpec{primitive.NewTrue()},
		Metrics:         metrics,
		SignalFirstLoad: true,
	})
	defer func() {
		r.Close()
		dc.Close()
		time.Sleep(100 * time.Millisecond) // Allow goroutines to clean up
	}()
	<-r.FirstLoad()

	// Verify the invalid route metric is set to 1
	time.Sleep(10 * time.Millisecond)
	metrics.WithGauges(func(gauges map[string]float64) {
		expectedKey := "route.invalid.invalidBackend1..failed_backend_split"
		if gauges[expectedKey] != 1 {
			t.Errorf("Expected invalid route metric to be 1, got %f", gauges[expectedKey])
		}
	})

	// Now fix the route by updating with a valid backend
	dc.UpdateDoc(`invalidBackend1: Path("/bad1") -> "https://example.org";`, nil)

	// Wait for the update to be processed
	time.Sleep(50 * time.Millisecond)

	// Verify the metric is now set to 0 (not deleted)
	metrics.WithGauges(func(gauges map[string]float64) {
		expectedKey := "route.invalid.invalidBackend1..failed_backend_split"
		if gauges[expectedKey] != 0 {
			t.Errorf("Expected invalid route metric to be set to 0 when route becomes valid, got %f", gauges[expectedKey])
		}
	})
}

func testRouteMetricsSetToZeroWhenFixedWithPrometheus(t *testing.T) {
	pm := metrics.NewPrometheus(metrics.Options{})
	path := "/metrics"

	mux := http.NewServeMux()
	pm.RegisterHandler(path, mux)

	// Start with an invalid route
	dc, err := testdataclient.NewDoc(`invalidBackend1: Path("/bad1") -> "invalid-url";`)
	if err != nil {
		t.Fatal(err)
	}

	fr := make(filters.Registry)
	fr.Register(builtin.NewSetPath())

	r := routing.New(routing.Options{
		DataClients:     []routing.DataClient{dc},
		FilterRegistry:  fr,
		Predicates:      []routing.PredicateSpec{primitive.NewTrue()},
		Metrics:         pm,
		SignalFirstLoad: true,
	})
	defer func() {
		r.Close()
		dc.Close()
		time.Sleep(100 * time.Millisecond) // Allow goroutines to clean up
	}()
	<-r.FirstLoad()

	// Verify the invalid route metric is set to 1
	time.Sleep(10 * time.Millisecond)
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	body, _ := io.ReadAll(w.Result().Body)
	output := string(body)

	expectedLine := `skipper_route_invalid{reason="failed_backend_split",route_id="invalidBackend1"} 1`
	if !strings.Contains(output, expectedLine) {
		t.Errorf("Expected to find invalid route metric set to 1")
		t.Logf("Output: %s", output)
	}

	// Now fix the route
	dc.UpdateDoc(`invalidBackend1: Path("/bad1") -> "https://example.org";`, nil)

	// Wait for the update to be processed
	time.Sleep(50 * time.Millisecond)

	// Check that the metric is now set to 0
	req = httptest.NewRequest("GET", path, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	body, _ = io.ReadAll(w.Result().Body)
	output = string(body)

	expectedZeroLine := `skipper_route_invalid{reason="failed_backend_split",route_id="invalidBackend1"} 0`
	if !strings.Contains(output, expectedZeroLine) {
		t.Errorf("Expected to find invalid route metric set to 0 when route becomes valid")
		t.Logf("Output: %s", output)
	}
}

type weightedPredicateSpec struct {
	name   string
	weight int
}
type weightedPredicate struct{}

func (w weightedPredicate) Match(request *http.Request) bool {
	return true
}

func (w weightedPredicateSpec) Name() string {
	return w.name
}

func (w weightedPredicateSpec) Create([]interface{}) (routing.Predicate, error) {
	return weightedPredicate{}, nil
}

func (w weightedPredicateSpec) Weight() int {
	return w.weight
}

type (
	slowCreateSpec   struct{}
	slowCreateFilter struct{}
)

func (s slowCreateSpec) Name() string { return "slowCreate" }

func (s slowCreateSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	d, _ := time.ParseDuration(args[0].(string))

	time.Sleep(d)

	return slowCreateFilter{}, nil
}

func (s slowCreateFilter) Request(ctx filters.FilterContext) {}
func (s slowCreateFilter) Response(filters.FilterContext)    {}
