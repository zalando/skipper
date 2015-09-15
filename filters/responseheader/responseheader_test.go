package responseheader

import (
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
	"net/http"
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
	c := &mock.FilterContext{nil, nil, r, false, make(skipper.StateBag)}
	f.Response(c)
	if r.Header.Get("X-Test") != "test-value" {
		t.Error("failed to set response header")
	}
}
