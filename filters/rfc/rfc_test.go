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

func TestPatchHost(t *testing.T) {
	req, err := http.NewRequest("GET", "http://www.example.org.", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &filtertest.Context{
		FRequest: req,
	}

	f, err := NewHost().CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	f.Request(ctx)
	if req.Host != "www.example.org" {
		t.Error("failed to patch the host", req.Host)
	}
}
