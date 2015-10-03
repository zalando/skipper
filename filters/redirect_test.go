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
	f, err := spec.MakeFilter("redirect0", []interface{}{float64(http.StatusFound), "https://example.org"})
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
