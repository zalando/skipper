package builtin

import (
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

func TestDropQuery(t *testing.T) {
	spec := NewDropQuery()
	f, err := spec.CreateFilter([]interface{}{"foo"})
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
	f, err := spec.CreateFilter([]interface{}{"${param1}"})
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
	f, err := spec.CreateFilter([]interface{}{"foo", "bar"})
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
		t.Error("failed to replace path")
	}
}

func TestSetQueryWithTemplate(t *testing.T) {
	spec := NewSetQuery()
	f, err := spec.CreateFilter([]interface{}{"${param2}", "${param1}"})
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
