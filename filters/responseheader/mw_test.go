package responseheader

import (
	"net/http"
	"github.com/zalando/skipper/mock"
	"testing"
)

func TestCreatesFilterSpec(t *testing.T) {
	mw := Make()
	if mw.Name() != "responseHeader" {
		t.Error("wrong name")
	}
}

func TestSetsResponseHeader(t *testing.T) {
	mw := Make()
	f, _ := mw.MakeFilter("filter", []interface{}{"X-Test", "test-value"})
	r := &http.Response{Header: make(http.Header)}
	c := &mock.FilterContext{nil, nil, r, false}
	f.Response(c)
	if r.Header.Get("X-Test") != "test-value" {
		t.Error("failed to set response header")
	}
}
