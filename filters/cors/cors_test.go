package cors

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestWithMissingOrigins(t *testing.T) {
	spec := NewOrigin()
	f, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Error(err)
	}

	expectedHeaderValue := "*"

	ctx := &filtertest.Context{FResponse: &http.Response{Header: http.Header{}}}
	f.Response(ctx)
	if ctx.Response().Header.Get(allowOriginHeader) != expectedHeaderValue {
		t.Error("origin header wrong/missing")
	}
}

func TestWithMissingOriginHeader(t *testing.T) {
	spec := NewOrigin()
	f, err := spec.CreateFilter([]interface{}{"https://www.example.org"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	if err != nil {
		t.Error(err)
	}

	expectedHeaderValue := ""

	ctx := &filtertest.Context{FRequest: req, FResponse: &http.Response{Header: http.Header{}}}
	f.Response(ctx)
	if ctx.Response().Header.Get(allowOriginHeader) != expectedHeaderValue {
		t.Error("origin header present when it should not")
	}
}

func TestWithOriginHeader(t *testing.T) {
	spec := NewOrigin()
	f, err := spec.CreateFilter([]interface{}{"https://www.example.org"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	if err != nil {
		t.Error(err)
	}

	req.Header.Set("Origin", "https://www.example.org")

	expectedHeaderValue := "https://www.example.org"

	ctx := &filtertest.Context{FRequest: req, FResponse: &http.Response{Header: http.Header{}}}
	f.Response(ctx)
	if ctx.Response().Header.Get(allowOriginHeader) != expectedHeaderValue {
		t.Error("origin header wrong/missing")
	}
}
