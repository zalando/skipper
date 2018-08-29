package filtertest

import (
	"context"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
)

func TestRequestContext(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "test", 1)
	req, _ := http.NewRequest("GET", "http://localhost:9090", nil)
	req = req.WithContext(ctx)
	fc := filters.FilterContext(&Context{FRequest: req})

	origVal := fc.Request().Context().Value("test").(int)

	fc.RequestContext(context.WithValue(fc.Request().Context(), "test", 2))

	newVal := fc.Request().Context().Value("test").(int)
	if newVal == origVal {
		t.Errorf("orig and new context value are the same...")
	}
	if newVal != 2 || origVal != 1 {
		t.Errorf("invalid value fetched from context")
	}
}
