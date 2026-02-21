package builtin

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestDropQuery(t *testing.T) {
	spec := NewDropQuery()
	f, err := spec.CreateFilter([]any{"foo"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path?foo=1&bar=2", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.String() != "https://www.example.org/path?bar=2" {
		t.Error("failed to replace path")
	}
}

func TestDropQueryWithTemplate(t *testing.T) {
	spec := NewDropQuery()
	f, err := spec.CreateFilter([]any{"${param1}"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path?foo=1&bar=2", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req, FParams: map[string]string{
		"param1": "foo",
	}}

	f.Request(ctx)
	if req.URL.String() != "https://www.example.org/path?bar=2" {
		t.Error("failed to transform path")
	}
}

func TestSetQuery(t *testing.T) {
	spec := NewSetQuery()
	f, err := spec.CreateFilter([]any{"foo", "bar"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path?foo=1", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.String() != "https://www.example.org/path?foo=bar" {
		t.Error("failed to replace query")
	}
}

func TestSetQueryKeyOnly(t *testing.T) {
	spec := NewSetQuery()
	f, err := spec.CreateFilter([]any{"foo"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path?foo", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.String() != "https://www.example.org/path?foo" {
		t.Error("failed to replace query")
	}
}

func TestSetQueryWithTemplate(t *testing.T) {
	spec := NewSetQuery()
	f, err := spec.CreateFilter([]any{"${param2}", "${param1}"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req, FParams: map[string]string{
		"param1": "foo",
		"param2": "bar",
	}}

	f.Request(ctx)
	if req.URL.String() != "https://www.example.org/path?bar=foo" {
		t.Error("failed to transform path")
	}
}
