package filters

import (
	"github.com/zalando/skipper/mock"
	"net/http/httptest"
	"testing"
)

func TestRedirect(t *testing.T) {
	spec := &Redirect{}
	f, err := spec.MakeFilter("redirect0", []interface{}{float64(302), "https://example.org"})
	if err != nil {
		t.Error(err)
	}

	ctx := &mock.FilterContext{FResponseWriter: httptest.NewRecorder()}
	f.Response(ctx)

	if ctx.FResponseWriter.(*httptest.ResponseRecorder).Code != 302 {
		t.Error("invalid status code")
	}

	if ctx.FResponseWriter.Header().Get("Location") != "https://example.org" {
		t.Error("invalid location")
	}
}
