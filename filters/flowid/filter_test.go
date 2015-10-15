package flowid

import (
	"github.com/zalando/skipper/mock"
	"net/http"
	"testing"
)

const testFlowId = "FLOW-ID-FOR-TESTING"

func TestNewFlowIdGeneration(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://example.org", nil)
	f := New(filterName, true)
	fc := &mock.FilterContext{FRequest: r}
	f.Request(fc)

	flowId := fc.Request().Header.Get(flowIdHeaderName)
	if !isValid(flowId) {
		t.Errorf("'%s' is not a valid flow id", flowId)
	}
}

func TestFlowIdReuseExisting(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://example.org", nil)
	f := New(filterName, true)
	r.Header.Set(flowIdHeaderName, testFlowId)
	fc := &mock.FilterContext{FRequest: r}
	f.Request(fc)

	flowId := fc.Request().Header.Get(flowIdHeaderName)
	if flowId != testFlowId {
		t.Errorf("Got wrong flow id. Expected '%s' got '%s'", testFlowId, flowId)
	}
}

func TestFlowIdIgnoreReuseExisting(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://example.org", nil)
	f := New(filterName, false)
	r.Header.Set(flowIdHeaderName, testFlowId)
	fc := &mock.FilterContext{FRequest: r}
	f.Request(fc)

	flowId := fc.Request().Header.Get(flowIdHeaderName)
	if flowId == testFlowId {
		t.Errorf("Got wrong flow id. Expected a newly generated flowid but got the test flow id '%s'", flowId)
	}
}

func TestFlowIdRejectInvalidFlowId(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://example.org", nil)
	f := New(filterName, true)
	r.Header.Set(flowIdHeaderName, "[<>] (o) [<>]")
	fc := &mock.FilterContext{FRequest: r}
	f.Request(fc)

	flowId := fc.Request().Header.Get(flowIdHeaderName)
	if flowId == "[<>] (o) [<>]" {
		t.Errorf("Got wrong flow id. Expected a newly generated flowid but got the test flow id '%s'", flowId)
	}
}
