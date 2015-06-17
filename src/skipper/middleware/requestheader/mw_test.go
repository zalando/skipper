package requestheader

import (
	"net/http"
	"testing"
)

func TestCreatesMiddleware(t *testing.T) {
	mw := Make()
	if mw.Name() != "request-header" {
		t.Error("wrong name")
	}
}

func TestCreatesFilter(t *testing.T) {
	mw := Make()
	f, err := mw.MakeFilter("filter", map[string]interface{}{"key": "X-Test", "value": "test-value"})
	if err != nil || f.Id() != "filter" {
		t.Error("failed to create filter")
	}
}

func TestReportsMissingKey(t *testing.T) {
	mw := Make()
	_, err := mw.MakeFilter("filter", map[string]interface{}{"value": "test-value"})
	if err == nil {
		t.Error("failed to fail on missing key")
	}
}

func TestReportsMissingValue(t *testing.T) {
	mw := Make()
	_, err := mw.MakeFilter("filter", map[string]interface{}{"key": "X-Test"})
	if err == nil {
		t.Error("failed to fail on missing value")
	}
}

func TestSetsRequestHeader(t *testing.T) {
	mw := Make()
	f, _ := mw.MakeFilter("filter", map[string]interface{}{"key": "X-Test", "value": "test-value"})
	r, _ := http.NewRequest("GET", "test:", nil)
	rb := f.ProcessRequest(r)
	if rb.Header.Get("X-Test") != "test-value" {
		t.Error("failed to set request header")
	}
}
