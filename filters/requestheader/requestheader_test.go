package requestheader

import (
	"github.com/zalando/skipper/mock"
	"net/http"
	"testing"
)

func TestCreatesFilterSpec(t *testing.T) {
	mw := Make()
	if mw.Name() != "requestHeader" {
		t.Error("wrong name")
	}
}

func TestCreatesFilter(t *testing.T) {
	mw := Make()
	f, err := mw.MakeFilter("filter", []interface{}{"X-Test", "test-value"})
	if err != nil || f.Id() != "filter" {
		t.Error("failed to create filter")
	}
}

func TestReportsMissingArg(t *testing.T) {
	mw := Make()
	_, err := mw.MakeFilter("filter", []interface{}{"test-value"})
	if err == nil {
		t.Error("failed to fail on missing key")
	}
}

func TestSetsRequestHeader(t *testing.T) {
	mw := Make()
	f, _ := mw.MakeFilter("filter", []interface{}{"X-Test", "test-value"})
	r, _ := http.NewRequest("GET", "test:", nil)
	c := &mock.FilterContext{nil, r, nil, false}
	f.Request(c)
	if r.Header.Get("X-Test") != "test-value" {
		t.Error("failed to set request header")
	}
}
