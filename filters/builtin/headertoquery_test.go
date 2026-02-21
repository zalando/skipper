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
