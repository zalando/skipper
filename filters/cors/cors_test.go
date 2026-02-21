package cors

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestWithMissingOrigins(t *testing.T) {
	spec := NewOrigin()
	f, err := spec.CreateFilter([]any{})
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
	f, err := spec.CreateFilter([]any{"https://www.example.org"})
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
	f, err := spec.CreateFilter([]any{"https://www.example.org"})
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

func TestSingleHeader(t *testing.T) {
	spec := NewOrigin()
	f, err := spec.CreateFilter([]any{"https://www.example.org"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	if err != nil {
		t.Error(err)
	}

	req.Header.Set("Origin", "https://www.example.org")

	ctx := &filtertest.Context{
		FRequest:  req,
		FResponse: &http.Response{Header: http.Header{allowOriginHeader: []string{"https://www.other-example.org"}}},
	}

	expectedHeaderValue := "https://www.example.org"

	f.Response(ctx)
	headers := ctx.Response().Header[allowOriginHeader]
	if len(headers) != 1 {
		t.Error("header should only contain one value")
	}
	if headers[0] != expectedHeaderValue {
		t.Error("backend header value should have been overwritten")
	}
}
