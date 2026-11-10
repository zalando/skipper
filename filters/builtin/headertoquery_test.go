package builtin

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestHeaderToQueryFilter_Request(t *testing.T) {
	spec := NewHeaderToQuery()
	f, err := spec.CreateFilter([]any{"X-Foo-Header", "foo-query-param"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("Get", "https://example.org", nil)
	if err != nil {
		t.Error(err)
	}
	req.Header.Add("X-Foo-Header", "bar")

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.String() != "https://example.org?foo-query-param=bar" {
		t.Error("failed to move header to query param", req.URL.String())
	}
}

func TestHeaderToQueryFilter_RequestWithoutHeader(t *testing.T) {
	spec := NewHeaderToQuery()
	f, err := spec.CreateFilter([]any{"X-Foo-Header", "foo-query-param"})
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range []string{
		"https://example.org",
		"https://example.org?foo-query-param=baz",
		"https://example.org?b=2&a=1",
	} {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			t.Fatal(err)
		}

		ctx := &filtertest.Context{FRequest: req}
		f.Request(ctx)
		if req.URL.String() != u {
			t.Errorf("request without the header: got %s, want %s unchanged", req.URL.String(), u)
		}
	}
}
