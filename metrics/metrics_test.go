package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/rcrowley/go-metrics"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	o := Options{}

	Init(o)

	if reg != nil {
		t.Error("Default Options should not create a registry or enable metrics")
	}

	timer := getTimer(KeyRouteLookup)
	if timer != nil {
		t.Errorf("Able to get metric timer for key '%s' while it shouldn't be possible", KeyRouteLookup)
	}

	counter := getCounter(KeyRouteFailure)
	if counter != nil {
		t.Errorf("Able to get metric counter for key '%s' while it shouldn't be possible")
	}
}

func TestDefaultOptionsWithListener(t *testing.T) {
	o := Options{Listener: ":0"}
	Init(o)

	if reg == nil {
		t.Error("Options containing a listener should create a registry")
	}

	if reg.Get("debug.GCStats.LastGC") != nil {
		t.Error("Default options should not enable debug gc stats")
	}

	if reg.Get("runtime.MemStats.Alloc") != nil {
		t.Error("Default options should not enable runtime stats")
	}
}

func TestDebugGcStats(t *testing.T) {
	o := Options{Listener: ":0", EnableDebugGcMetrics: true}
	Init(o)

	if reg.Get("debug.GCStats.LastGC") == nil {
		t.Error("Options enabled debug gc stats but failed to find the key 'debug.GCStats.LastGC'")
	}
}

func TestRuntimeStats(t *testing.T) {
	o := Options{Listener: ":0", EnableRuntimeMetrics: true}
	Init(o)

	if reg.Get("runtime.MemStats.Alloc") == nil {
		t.Error("Options enabled runtime stats but failed to find the key 'runtime.MemStats.Alloc'")
	}
}

func TestMeasurement(t *testing.T) {
	o := Options{Listener: ":0"}
	Init(o)

	t1 := getTimer("TestMeasurement1")
	if t1.Count() != 0 && t1.Max() != 0 {
		t.Error("'TestMeasurement1' metric should only have zeroes")
	}
	now := time.Now()
	time.Sleep(5)
	measureSince("TestMeasurement1", now)

	time.Sleep(20 * time.Millisecond)
	if t1.Count() == 0 || t1.Max() == 0 {
		t.Error("'TestMeasurement1' metric should have some numbers")
	}

	t2 := getTimer("TestMeasurement2")
	if t2.Count() != 0 && t2.Max() != 0 {
		t.Error("'TestMeasurement2' metric should only have zeroes")
	}

	measureSince("TestMeasurement2", now)
	time.Sleep(20 * time.Millisecond)

	if t2.Count() == 0 || t2.Max() == 0 {
		t.Error("'TestMeasurement2' metric should have some numbers")
	}

	c1 := getCounter("TestCounter1")
	if c1.Count() != 0 {
		t.Error("'TestCounter1' metric should be zero")
	}
	incCounter("TestCounter1")
	time.Sleep(20 * time.Millisecond)
	if c1.Count() != 1 {
		t.Errorf("'TestCounter1' metric should be 1. Got %d", c1.Count())
	}
}

type proxyMetricTest struct {
	metricsKey  string
	measureFunc func()
}

var proxyMetricsTests = []proxyMetricTest{
	// T1 - Measure routing
	{KeyRouteLookup, func() { MeasureRouteLookup(time.Now()) }},
	// T2 - Measure filter request
	{fmt.Sprintf(KeyFilterRequest, "foo"), func() { MeasureFilterRequest("foo", time.Now()) }},
	// T3 - Measure all filters request
	{fmt.Sprintf(KeyFiltersRequest, "bar"), func() { MeasureAllFiltersRequest("bar", time.Now()) }},
	// T4 - Measure proxy backend
	{fmt.Sprintf(KeyProxyBackend, "baz"), func() { MeasureBackend("baz", time.Now()) }},
	// T5 - Measure filters response
	{fmt.Sprintf(KeyFilterResponse, "qux"), func() { MeasureFilterResponse("qux", time.Now()) }},
	// T6 - Measure all filters response
	{fmt.Sprintf(KeyFiltersResponse, "quux"), func() { MeasureAllFiltersResponse("quux", time.Now()) }},
	// T7 - Measure response
	{fmt.Sprintf(KeyResponse, http.StatusOK, "GET", "norf"),
		func() { MeasureResponse(http.StatusOK, "GET", "norf", time.Now()) }},
	// T8 - Inc routing failure
	{KeyRouteFailure, func() { IncRoutingFailures() }},
	// T9 - Inc backend errors
	{fmt.Sprintf(KeyErrorsBackend, "r1"), func() { IncErrorsBackend("r1") }},
	// T10 - Inc streaming errors
	{fmt.Sprintf(KeyErrorsStreaming, "r1"), func() { IncErrorsStreaming("r1") }},
}

func TestProxyMetrics(t *testing.T) {
	for _, pmt := range proxyMetricsTests {
		Init(Options{Listener: ":0"})
		pmt.measureFunc()
		reg.Each(func(key string, _ interface{}) {
			if key != pmt.metricsKey {
				t.Errorf("Registry contained unexpected metric for key '%s'. Found '%s'", pmt.metricsKey, key)
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

func TestMetricSerialization(t *testing.T) {
	metrics.UseNilMetrics = true
	for _, st := range serializationTests {
		m := reflect.ValueOf(st.i).Call(nil)[0].Interface()
		metrics := skipperMetrics{"test": m}
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(metrics)
		var got serializationResult
		json.Unmarshal(buf.Bytes(), &got)

		if !reflect.DeepEqual(got, st.expected) {
			t.Errorf("Got wrong serialization result. Expected '%v' but got '%v'", st.expected, got)
		}

	}
}
