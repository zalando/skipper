package builtin

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func TestCookieToHeader_Request(t *testing.T) {
	for _, ti := range []struct {
		name     string
		params   []interface{}
		expected string
	}{
		{
			name:     "simple",
			params:   []interface{}{"jwt", "Authorization"},
			expected: "verysecure",
		},
		{
			name:     "with prefix",
			params:   []interface{}{"jwt", "Authorization", "Bearer "},
			expected: "Bearer verysecure",
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			spec := NewCookieToHeader()
			f, err := spec.CreateFilter(ti.params)
			if err != nil {
				t.Error(err)
			}

			req, err := http.NewRequest("Get", "https://example.org", nil)
			if err != nil {
				t.Error(err)
			}
			req.AddCookie(&http.Cookie{Name: "jwt", Value: "verysecure"})

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)
			if req.Header.Get("Authorization") != ti.expected {
				t.Error("failed to move authorization cookie to authorization header:", req.Header.Get("Authorization"))
			}
		})
	}
}
