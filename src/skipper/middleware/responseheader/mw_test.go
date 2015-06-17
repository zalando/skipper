package responseheader

import (
	"net/http"
	"testing"
)

func TestCreatesMiddleware(t *testing.T) {
	mw := Make()
	if mw.Name() != "response-header" {
		t.Error("wrong name")
	}
}

func TestSetsResponseHeader(t *testing.T) {
	mw := Make()
	f, _ := mw.MakeFilter("filter", map[string]interface{}{"key": "X-Test", "value": "test-value"})
	r := &http.Response{Header: make(http.Header)}
	rb := f.ProcessResponse(r)
	if rb.Header.Get("X-Test") != "test-value" {
		t.Error("failed to set response header")
	}
}
