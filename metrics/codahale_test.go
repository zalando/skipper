package metrics

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rcrowley/go-metrics"
)

func TestUseVoidByDefaultOptions(t *testing.T) {
	if Default != Void {
		t.Error("Default Options should not create a registry or enable metrics")
	}

	c, ok := Default.(*CodaHale)
	if !ok {
		t.Error("Default metrics backend should be CodaHale")
	}
	timer := c.getTimer(KeyRouteLookup)
	switch timer.(type) {
	case metrics.NilTimer:
	default:
		t.Errorf("Able to get metric timer for key '%s' while it shouldn't be possible", KeyRouteLookup)
	}

	counter := c.getCounter(KeyRouteFailure)
	switch counter.(type) {
	case metrics.NilCounter:
	default:
		t.Errorf("Able to get metric counter for key '%s' while it shouldn't be possible", KeyRouteFailure)
	}
}

func TestDefaultOptionsWithListener(t *testing.T) {
	o := Options{}
	c := NewCodaHale(o)
	defer c.Close()

	if c == Void {
		t.Error("Options containing a listener should create a registry")
	}

	if c.reg.Get("debug.GCStats.LastGC") != nil {
		t.Error("Default options should not enable debug gc stats")
	}

	if c.reg.Get("runtime.MemStats.Alloc") != nil {
		t.Error("Default options should not enable runtime stats")
	}
}

func TestCodaHaleDebugGcStats(t *testing.T) {
	o := Options{EnableDebugGcMetrics: true}
	c := NewCodaHale(o)
	defer c.Close()

	if c.reg.Get("debug.GCStats.LastGC") == nil {
		t.Error("Options enabled debug gc stats but failed to find the key 'debug.GCStats.LastGC'")
	}
}

func TestCodaHaleRuntimeStats(t *testing.T) {
	o := Options{EnableRuntimeMetrics: true}
	c := NewCodaHale(o)
	defer c.Close()

	if c.reg.Get("runtime.MemStats.Alloc") == nil {
		t.Error("Options enabled runtime stats but failed to find the key 'runtime.MemStats.Alloc'")
	}
}

func TestCodaHaleMeasurement(t *testing.T) {
	o := Options{}
	c := NewCodaHale(o)
	defer c.Close()

	g1 := c.getGauge("TestGauge")
	c.UpdateGauge("TestGauge", 1)
	c.UpdateGauge("TestGauge", 2)
	c.UpdateGauge("TestGauge", 3)
	if g1.Value() < 3 {
		t.Errorf("'TestGauge' metric should be 1. Got %f", g1.Value())
	}

	t1 := c.getTimer("TestMeasurement1")
	if t1.Count() != 0 && t1.Max() != 0 {
		t.Error("'TestMeasurement1' metric should only have zeroes")
	}
	now := time.Now()
	time.Sleep(5 * time.Nanosecond)
	c.measureSince("TestMeasurement1", now)

	time.Sleep(20 * time.Millisecond)
	if t1.Count() == 0 || t1.Max() == 0 {
		t.Error("'TestMeasurement1' metric should have some numbers")
	}

	t2 := c.getTimer("TestMeasurement2")
	if t2.Count() != 0 && t2.Max() != 0 {
		t.Error("'TestMeasurement2' metric should only have zeroes")
	}

	c.measureSince("TestMeasurement2", now)
	time.Sleep(20 * time.Millisecond)

	if t2.Count() == 0 || t2.Max() == 0 {
		t.Error("'TestMeasurement2' metric should have some numbers")
	}

	c1 := c.getCounter("TestCounter1")
	if c1.Count() != 0 {
		t.Error("'TestCounter1' metric should be zero")
	}
	c.incCounter("TestCounter1", 1)
	time.Sleep(20 * time.Millisecond)
	if c1.Count() != 1 {
		t.Errorf("'TestCounter1' metric should be 1. Got %d", c1.Count())
	}

	c2 := c.getCounter("TestCounter2")
	if c2.Count() != 0 {
		t.Error("'TestCounter2' metric should be zero")
	}
	c.IncCounter("TestCounter2")
	time.Sleep(20 * time.Millisecond)
	if c2.Count() != 1 {
		t.Errorf("'TestCounter2' metric should be 1. Got %d", c2.Count())
	}

	t3 := c.getTimer("TestMeasurement3")
	if t3.Count() != 0 && t3.Max() != 0 {
		t.Error("'TestMeasurement3' metric should only have zeroes")
	}

	c.MeasureSince("TestMeasurement3", now)
	time.Sleep(20 * time.Millisecond)

	if t3.Count() == 0 || t3.Max() == 0 {
		t.Error("'TestMeasurement2' metric should have some numbers")
	}

}

type proxyMetricTest struct {
	metricsKey  string
	measureFunc func(Metrics)
}

var proxyMetricsTests = []proxyMetricTest{
	// T1 - Measure routing
	{KeyRouteLookup, func(m Metrics) { m.MeasureRouteLookup(time.Now()) }},
	{fmt.Sprintf(KeyFilterCreate, "afilter"), func(m Metrics) { m.MeasureFilterCreate("afilter", time.Now()) }},
	// T2 - Measure filter request
	{fmt.Sprintf(KeyFilterRequest, "foo"), func(m Metrics) { m.MeasureFilterRequest("foo", time.Now()) }},
	// T3 - Measure all filters request
	{fmt.Sprintf(KeyFiltersRequest, "bar"), func(m Metrics) { m.MeasureAllFiltersRequest("bar", time.Now()) }},
	// T4 - Measure proxy backend
	{fmt.Sprintf(KeyProxyBackend, "baz"), func(m Metrics) { m.MeasureBackend("baz", time.Now()) }},
	// T5 - Measure filters response
	{fmt.Sprintf(KeyFilterResponse, "qux"), func(m Metrics) { m.MeasureFilterResponse("qux", time.Now()) }},
	// T6 - Measure all filters response
	{fmt.Sprintf(KeyFiltersResponse, "quux"), func(m Metrics) { m.MeasureAllFiltersResponse("quux", time.Now()) }},
	// T7 - Measure response
	{fmt.Sprintf(KeyResponse, http.StatusOK, "GET", "norf"),
		func(m Metrics) { m.MeasureResponse(http.StatusOK, "GET", "norf", time.Now()) }},
	// T8 - Inc routing failure
	{KeyRouteFailure, func(m Metrics) { m.IncRoutingFailures() }},
	// T9 - Inc backend errors
	{fmt.Sprintf(KeyErrorsBackend, "r1"), func(m Metrics) { m.IncErrorsBackend("r1") }},
	// T10 - Inc streaming errors
	{fmt.Sprintf(KeyErrorsStreaming, "r1"), func(m Metrics) { m.IncErrorsStreaming("r1") }},
}

func TestCodaHaleProxyMetrics(t *testing.T) {
	for _, pmt := range proxyMetricsTests {
		t.Run(pmt.metricsKey, func(t *testing.T) {
			m := NewCodaHale(Options{})
			defer m.Close()

			pmt.measureFunc(m)

			time.Sleep(20 * time.Millisecond)
			if m.reg.Get(pmt.metricsKey) == nil {
				t.Errorf("expected metric was not found: '%s'", pmt.metricsKey)
			}
		})
	}
}

type serializationResult map[string]map[string]map[string]interface{}

type serializationTest struct {
	i        interface{}
	expected serializationResult
}

var serializationTests = []serializationTest{
	{metrics.NewGauge, serializationResult{"gauges": {"test": {"value": 0.0}}}},
	{metrics.NewCounter, serializationResult{"counters": {"test": {"count": 0.0}}}},
	{metrics.NewTimer, serializationResult{"timers": {"test": {"15m.rate": 0.0, "1m.rate": 0.0, "5m.rate": 0.0,
		"75%": 0.0, "95%": 0.0, "99%": 0.0, "99.9%": 0.0, "count": 0.0, "max": 0.0, "mean": 0.0, "mean.rate": 0.0,
		"median": 0.0, "min": 0.0, "stddev": 0.0}}}},
	{func() metrics.Histogram { return metrics.NewHistogram(nil) }, serializationResult{"histograms": {"test": {"75%": 0.0,
		"95%": 0.0, "99%": 0.0, "99.9%": 0.0, "count": 0.0, "max": 0.0, "mean": 0.0, "median": 0.0, "min": 0.0,
		"stddev": 0.0}}}},
	{func() int { return 42 }, serializationResult{"unknown": {"test": {"error": "unknown metrics type int"}}}},
}

type serveMetricsMeasure struct {
	route, host, method string
	status              int
	duration            time.Duration
}

type serveMetricsCheck struct {
	key         string
	enabled     bool
	count       int64
	minDuration time.Duration
}

type serveMetricsTestItem struct {
	msg      string
	options  Options
	measures []serveMetricsMeasure
	checks   []serveMetricsCheck
}

func TestCodaHaleServeMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	for _, ti := range []serveMetricsTestItem{{
		"route and host disabled",
		Options{},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:     "serveroute.route1.GET.200",
			enabled: false,
		}, {
			key:     "servehost.www_example_org__4443.GET.200",
			enabled: false,
		}},
	}, {
		"route enabled, host disabled",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeMethodMetric:     true,
			EnableServeStatusCodeMetric: true,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:     "servehost.www_example_org__4443.GET.200",
			enabled: false,
		}},
	}, {
		"route disabled, host enabled",
		Options{
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     true,
			EnableServeStatusCodeMetric: true,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:     "serveroute.route1.GET.200",
			enabled: false,
		}, {
			key:         "servehost.www_example_org__4443.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}, {
		"route and host enabled",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     true,
			EnableServeStatusCodeMetric: true,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}, {
		"route and host enabled without method label",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     false,
			EnableServeStatusCodeMetric: true,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}, {
		"route and host enabled without status code label",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     true,
			EnableServeStatusCodeMetric: false,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1.GET",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443.GET",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}, {
		"route and host enabled without status code and method label",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     false,
			EnableServeStatusCodeMetric: false,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}, {
		"collect different metrics",
		Options{
			EnableServeRouteMetrics:     true,
			EnableServeHostMetrics:      true,
			EnableServeMethodMetric:     true,
			EnableServeStatusCodeMetric: true,
		},
		[]serveMetricsMeasure{{
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}, {
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			15 * time.Millisecond,
		}, {
			"route1",
			"www.example.org:4443",
			"GET",
			200,
			30 * time.Millisecond,
		}, {
			"route2",
			"www.example.org",
			"GET",
			200,
			30 * time.Millisecond,
		}, {
			"route1",
			"www.example.org:4443",
			"POST",
			302,
			30 * time.Millisecond,
		}},
		[]serveMetricsCheck{{
			key:         "serveroute.route1.GET.200",
			enabled:     true,
			count:       3,
			minDuration: 15 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443.GET.200",
			enabled:     true,
			count:       3,
			minDuration: 15 * time.Millisecond,
		}, {
			key:         "serveroute.route2.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org.GET.200",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "serveroute.route1.POST.302",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}, {
			key:         "servehost.www_example_org__4443.POST.302",
			enabled:     true,
			count:       1,
			minDuration: 30 * time.Millisecond,
		}},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			checkMetrics := func(m *CodaHale, key string, enabled bool, count int64, minDuration time.Duration) (bool, string) {
				v := m.reg.Get(key)

				switch {
				case enabled && v == nil:
					return false, "metric not found in the registry"
				case !enabled && v != nil:
					return false, "unexpected metrics"
				case !enabled && v == nil:
					return true, ""
				}

				tr, ok := v.(metrics.Timer)
				if !ok {
					return false, "invalid metric type"
				}

				trs := tr.Snapshot()

				if trs.Count() != count {
					return false, fmt.Sprintf("failed to get the right count: %d instead of %d", trs.Count(), count)
				}

				if trs.Min() <= int64(minDuration) {
					return false, "failed to get the right duration"
				}

				return true, ""
			}

			m := NewCodaHale(ti.options)
			defer m.Close()

			for _, mi := range ti.measures {
				m.MeasureServe(mi.route, mi.host, mi.method, mi.status, time.Now().Add(-mi.duration))
			}

			time.Sleep(12 * time.Millisecond)
			for _, ci := range ti.checks {
				if ok, reason := checkMetrics(
					m,
					ci.key,
					ci.enabled,
					ci.count,
					ci.minDuration,
				); !ok {
					t.Errorf("error processing metric '%s': %s", ci.key, reason)
					return
				}
			}
		})
	}
}

type latencyMetricsTestItem struct {
	msg              string
	requestDuration  time.Duration
	responseDuration time.Duration
	enabledRequest   bool
	enabledResponse  bool
}

func TestCodaHaleProxyLatencyMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	for _, ti := range []latencyMetricsTestItem{
		{
			msg:              "request and response disabled",
			requestDuration:  100 * time.Millisecond,
			responseDuration: 200 * time.Millisecond,
			enabledRequest:   false,
			enabledResponse:  false,
		},
		{
			msg:              "request enabled, response disabled",
			requestDuration:  10 * time.Millisecond,
			responseDuration: 20 * time.Millisecond,
			enabledRequest:   true,
			enabledResponse:  false,
		},
		{
			msg:              "request disabled, response enabled",
			requestDuration:  300 * time.Millisecond,
			responseDuration: 100 * time.Millisecond,
			enabledRequest:   false,
			enabledResponse:  true,
		},
		{
			msg:              "request and response enabled",
			requestDuration:  150 * time.Millisecond,
			responseDuration: 200 * time.Millisecond,
			enabledRequest:   true,
			enabledResponse:  true,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			checkMetrics := func(m *CodaHale, key string, enabled bool, count int64, duration time.Duration) (bool, string) {
				v := m.reg.Get(key)

				switch {
				case enabled && v == nil:
					return false, "metric not found in the registry"
				case !enabled && v != nil:
					return false, "unexpected metrics"
				case !enabled && v == nil:
					return true, ""
				}

				tr, ok := v.(metrics.Timer)
				if !ok {
					return false, "invalid metric type"
				}

				trs := tr.Snapshot()

				if trs.Count() != count {
					return false, fmt.Sprintf("failed to get the right count: %d instead of %d", trs.Count(), count)
				}

				if trs.Max() > int64(duration) || trs.Min() < int64(duration) {
					return false, "failed to get the right duration"
				}

				return true, ""
			}

			o := Options{
				EnableProxyRequestMetrics:  ti.enabledRequest,
				EnableProxyResponseMetrics: ti.enabledResponse,
			}
			c := NewCodaHale(o)
			defer c.Close()

			c.MeasureProxy(ti.requestDuration, ti.responseDuration)
			time.Sleep(10 * time.Millisecond)

			if ok, reason := checkMetrics(c, KeyProxyTotal, true, 1, ti.requestDuration+ti.responseDuration); !ok {
				t.Errorf("error processing metric '%s': %s", KeyProxyTotal, reason)
			}
			if ok, reason := checkMetrics(c, KeyProxyRequest, ti.enabledRequest, 1, ti.requestDuration); !ok {
				t.Errorf("error processing metric '%s': %s", KeyProxyRequest, reason)
			}
			if ok, reason := checkMetrics(c, KeyProxyResponse, ti.enabledResponse, 1, ti.responseDuration); !ok {
				t.Errorf("error processing metric '%s': %s", KeyProxyResponse, reason)
			}
		})
	}
}
