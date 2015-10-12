package filters_test

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

func TestNoConfig(t *testing.T) {
	spec := &filters.ModPath{}
	_, err := spec.CreateFilter(nil)
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestInvalidConfig(t *testing.T) {
	spec := &filters.ModPath{}
	_, err := spec.CreateFilter([]interface{}{"invalid regexp: }*", 42})
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestModifyPath(t *testing.T) {
	spec := &filters.ModPath{}
	f, err := spec.CreateFilter([]interface{}{"/replace-this/", "/with-this/"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path/replace-this/yo", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.Path != "/path/with-this/yo" {
		t.Error("failed to replace path")
	}
}
