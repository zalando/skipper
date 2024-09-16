package normalizepath

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

// TestNormalizePath tests the NormalizePath function in accordance to the
// https://opensource.zalando.com/restful-api-guidelines/#136
// specifically the notion of normalization of request paths:
//
// All services should normalize request paths before processing by removing
// duplicate and trailing slashes. Hence, the following requests should refer
// to the same resource:
// GET /orders/{order-id}
// GET /orders/{order-id}/
// GET /orders//{order-id}
func TestNormalizePath(t *testing.T) {
	urls := []string{
		"/orders/{order-id}",
		"/orders/{order-id}/",
		"/orders//{order-id}",
		"/orders/{order-id}//",
		"/orders/{order-id}///",
		"/orders///{order-id}//",
	}

	for _, u := range urls {
		req := &http.Request{URL: &url.URL{Path: u}}
		ctx := &filtertest.Context{
			FRequest: req,
		}
		f, err := NewNormalizePath().CreateFilter(nil)
		if err != nil {
			t.Fatal(err)
		}
		f.Request(ctx)
		if req.URL.Path != "/orders/{order-id}" {
			t.Errorf("failed to normalize the path: %s", req.URL.Path)
		}
	}

	// Ensure that root paths work as expected
	req := &http.Request{URL: &url.URL{Path: "/"}}
	ctx := &filtertest.Context{
		FRequest: req,
	}
	f, err := NewNormalizePath().CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.Request(ctx)
	if req.URL.Path != "/" {
		t.Errorf("unexpected URL path change: %s, expected /", req.URL.Path)
	}
}
