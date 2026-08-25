package builtin

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestQueryToHeaderFilter_Request(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		args     []any
		expected string
	}{{
		msg:      "2 valid args",
		args:     []any{"foo", "X-Foo-Header"},
		expected: "bar",
	}, {
		msg:      "3 valid args",
		args:     []any{"foo", "X-Foo-Header", "MyPrefix %s"},
		expected: "MyPrefix bar",
	}} {
		t.Run(ti.msg, func(t *testing.T) {

			spec := NewQueryToHeader()
			f, err := spec.CreateFilter(ti.args)
			if err != nil {
				t.Error(err)
			}

			u := fmt.Sprintf("https://example.org?%s=bar", ti.args[0])
			req, err := http.NewRequest("Get", u, nil)
			if err != nil {
				t.Error(err)
			}

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)
			if headerKey, ok := ti.args[1].(string); !ok {
				t.Errorf("failed to cast to string %v", ti.args[1])
			} else if v := req.Header.Get(headerKey); v != ti.expected {
				t.Errorf("failed to copy query to header, expected='%s', but got value='%s'", ti.expected, v)
			}
		})
	}
}
