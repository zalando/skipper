package rfc

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestPatch(t *testing.T) {
	req := &http.Request{URL: &url.URL{RawPath: "/foo%2Fbar", Path: "/foo/bar"}}
	ctx := &filtertest.Context{
		FRequest: req,
	}

	f, err := NewPath().CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	f.Request(ctx)
	if req.URL.Path != "/foo%2Fbar" {
		t.Error("failed to patch the path", req.URL.Path)
	}
}
