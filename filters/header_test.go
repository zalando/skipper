package filters_test

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

func TestRequestHeader(t *testing.T) {
	spec := filters.CreateRequestHeader()
	if spec.Name() != "requestHeader" {
		t.Error("invalid name")
	}

	f, err := spec.CreateFilter([]interface{}{"Some-Header", "some-value"})
	if err != nil {
		t.Error(err)
	}

	r, err := http.NewRequest("GET", "test:", nil)
	if err != nil {
		t.Error(err)
	}

	c := &filtertest.Context{nil, r, nil, false, nil, nil}
	f.Request(c)
	if r.Header.Get("Some-Header") != "some-value" {
		t.Error("failed to set request header")
	}
}

func TestRequestHeaderInvalidConfigLength(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestRequestHeaderInvalidConfigKey(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{1, "some-value"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestRequestHeaderInvalidConfigValue(t *testing.T) {
	spec := filters.CreateRequestHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header", 2})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeader(t *testing.T) {
	spec := filters.CreateResponseHeader()
	if spec.Name() != "responseHeader" {
		t.Error("invalid name")
	}

	f, err := spec.CreateFilter([]interface{}{"Some-Header", "some-value"})
	if err != nil {
		t.Error(err)
	}

	r := &http.Response{Header: make(http.Header)}
	c := &filtertest.Context{nil, nil, r, false, nil, nil}
	f.Response(c)
	if r.Header.Get("Some-Header") != "some-value" {
		t.Error("failed to set request header")
	}
}

func TestResponseHeaderInvalidConfigLength(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeaderInvalidConfigKey(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{1, "some-value"})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestResponseHeaderInvalidConfigValue(t *testing.T) {
	spec := filters.CreateResponseHeader()
	_, err := spec.CreateFilter([]interface{}{"Some-Header", 2})
	if err == nil {
		t.Error("failed to fail")
	}
}
