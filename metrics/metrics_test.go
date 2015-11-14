package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	o := Options{}

	Init(o)

	if reg != nil {
		t.Error("Default Options should not create a registry or enable metrics")
	}

	timer := getTimer(KeyRouting)
	if timer != nil {
		t.Errorf("Able to get metric timer for key '%s' and it shouldn't be possible", KeyRouting)
	}

	var test = 42
	measure(KeyRouting, func() {
		test = 1
	})

	if test != 1 {
		t.Error("Failed to execute timed function without a registry")
	}

}

func TestDefaultOptionsWithAListener(t *testing.T) {
	o := Options{Listener: ":0"}
	Init(o)

	if reg == nil {
		t.Error("Options containing a listener should create a registry")
	}

	if !strings.HasPrefix(KeyRouting, "skipper.") {
		t.Error("Metric key has been prefixed. Expected to start with 'skipper.'")
	}

	if reg.Get("debug.GCStats.LastGC") != nil {
		t.Error("Default options should not enable debug gc stats")
	}

	if reg.Get("runtime.MemStats.Alloc") != nil {
		t.Error("Default options should not enable runtime stats")
	}
}

func TestMetricsPrefix(t *testing.T) {
	o := Options{Listener: ":0", Prefix: "test."}
	Init(o)

	if !strings.HasPrefix(KeyRouting, "test.") {
		t.Errorf("Metrics key should have the 'test.' prefix. Got '%s'", KeyRouting)
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

	if t1.Count() == 0 || t1.Max() == 0 {
		t.Error("'TestMeasurement1' metric should have some numbers")
	}

	t2 := getTimer("TestMeasurement2")
	if t2.Count() != 0 && t2.Max() != 0 {
		t.Error("'TestMeasurement2' metric should only have zeroes")
	}

	measure("TestMeasurement2", func() {
		time.Sleep(5)
	})

	if t2.Count() == 0 || t2.Max() == 0 {
		t.Error("'TestMeasurement2' metric should have some numbers")
	}
}

func TestHttpEndpoint(t *testing.T) {
	o := Options{Listener: ":0", EnableRuntimeMetrics: true}
	Init(o)
	r1, _ := http.NewRequest("GET", "/", nil)
	rw1 := httptest.NewRecorder()

	mh := new(metricsHandler)

	mh.ServeHTTP(rw1, r1)
	if rw1.Code != http.StatusBadRequest {
		t.Error("The root resource should not provide a valid response")
	}

	r2, _ := http.NewRequest("POST", "/metrics", nil)
	rw2 := httptest.NewRecorder()
	mh.ServeHTTP(rw2, r2)
	if rw2.Code != http.StatusBadRequest {
		t.Error("POST method should not provide a valid response")
	}

	r3, _ := http.NewRequest("GET", "/metrics", nil)
	rw3 := httptest.NewRecorder()
	mh.ServeHTTP(rw3, r3)
	if rw3.Code != http.StatusOK {
		t.Error("Metrics endpoint should provide a valid response")
	}

	if !strings.Contains(rw3.Body.String(), "runtime.") {
		t.Error("Metrics endpoint should've returned some runtime metrics in it")
	}

}
