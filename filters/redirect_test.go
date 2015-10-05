package filters_test

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirect(t *testing.T) {
	spec := &filters.Redirect{}
	f, err := spec.CreateFilter([]interface{}{float64(http.StatusFound), "https://example.org"})
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FResponseWriter: httptest.NewRecorder()}
	f.Response(ctx)

	if ctx.FResponseWriter.(*httptest.ResponseRecorder).Code != http.StatusFound {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != "https://example.org" {
		t.Error("invalid location")
	}
}

func TestRedirectRelative(t *testing.T) {
	spec := &filters.Redirect{}
	f, err := spec.CreateFilter([]interface{}{float64(http.StatusFound), "/relative/url"})
	if err != nil {
		t.Error(err)
	}

	request, _ := http.NewRequest("GET", "https://example.org/some/url", nil)

	ctx := &filtertest.Context{
		FResponseWriter: httptest.NewRecorder(),
		FRequest:        request}
	f.Response(ctx)

	if ctx.FResponseWriter.(*httptest.ResponseRecorder).Code != http.StatusFound {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != "https://example.org/relative/url" {
		t.Error("invalid location")
	}
}
