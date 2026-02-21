package builtin

import (
	"maps"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

type createTestItemHost struct {
	msg  string
	args []any
	err  bool
}

func TestModRequestHostHeader(t *testing.T) {
	for _, tt := range []struct {
		msg           string
		expression    string
		replacement   string
		url           string
		requestHeader http.Header
		expectedHost  string
	}{{
		"replace when Host header is provided and pattern matches",
		`^(example\.\w+)$`,
		`www.$1`,
		"https://example.org/path/yo",
		http.Header{"Host": []string{"example.org"}},
		"www.example.org",
	}, {
		"replace when Host header is not provided and pattern matches url host",
		`^(example\.\w+)$`,
		`www.$1`,
		"https://example.org/path/yo",
		http.Header{},
		"www.example.org",
	}, {
		"replace when Host header is not provided and pattern does not match url host",
		`^(example\.\w+)$`,
		`www.$1`,
		"https://zalando.de/path/yo",
		http.Header{},
		"zalando.de",
	}, {
		"replace when Host header is provided and pattern does not match",
		`^zalando\.de$`,
		`zalando.com`,
		"https://example.org/path/yo",
		http.Header{"Host": []string{"example.org"}},
		"example.org",
	}} {
		t.Run(tt.msg, func(t *testing.T) {
			spec := NewModRequestHeader()
			f, err := spec.CreateFilter([]any{"Host", tt.expression, tt.replacement})
			if err != nil {
				t.Error(err)
			}

			req, err := http.NewRequest("GET", tt.url, nil)

			if err != nil {
				t.Error(err)
			}

			maps.Copy(req.Header, tt.requestHeader)

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			if ctx.OutgoingHost() != tt.expectedHost {
				t.Errorf(`failed to modify OutgoingHost to "%s". Got: "%s"`, tt.expectedHost, ctx.OutgoingHost())
			}

			hv := req.Header.Get("Host")
			if hv != tt.expectedHost {
				t.Errorf(`failed to modify Host header to "%s". Got: "%s"`, tt.expectedHost, hv)
			}
		})
	}
}

func TestModRequestHeader(t *testing.T) {
	for _, tt := range []struct {
		msg                    string
		headerName             string
		expression             string
		replacement            string
		requestHeader          http.Header
		expectedHeader         string
		expectHeaderToNotExist bool
	}{{
		"replace when header is provided and pattern matches",
		"Accept-Language",
		`^nl\-NL$`,
		`en`,
		http.Header{"Accept-Language": []string{"nl-NL"}},
		"en",
		false,
	}, {
		"replace when header is provided and pattern matches anything",
		"Accept-Language",
		`^.*`,
		`en`,
		http.Header{"Accept-Language": []string{"nl-NL"}},
		"en",
		false,
	}, {
		"replace when header is not provided and pattern matches anything",
		"Accept-Language",
		`^.*`,
		`en`,
		http.Header{},
		"",
		true,
	}, {
		"do not replace when header is not provided and pattern does not match",
		"Accept-Language",
		`fr`,
		`en`,
		http.Header{},
		"",
		true,
	}} {
		t.Run(tt.msg, func(t *testing.T) {
			spec := NewModRequestHeader()
			f, err := spec.CreateFilter([]any{tt.headerName, tt.expression, tt.replacement})
			if err != nil {
				t.Error(err)
			}

			req, err := http.NewRequest("GET", "https://example.org/path/yo", nil)

			if err != nil {
				t.Error(err)
			}

			maps.Copy(req.Header, tt.requestHeader)

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			hv := req.Header.Get(tt.headerName)
			if hv != tt.expectedHeader {
				t.Errorf(`failed to modify request header %s to "%s". Got: "%s"`, tt.headerName, tt.expectedHeader, hv)
			}

			if tt.expectHeaderToNotExist && len(req.Header.Values(tt.headerName)) != 0 {
				t.Errorf(`expected header %s to not exist, but it does`, tt.headerName)
			}
		})
	}
}

func TestModResponseHeader(t *testing.T) {
	for _, tt := range []struct {
		msg                    string
		headerName             string
		expression             string
		replacement            string
		responseHeader         http.Header
		expectedHeader         string
		expectHeaderToNotExist bool
	}{{
		"replace when header is provided and pattern matches",
		"Accept-Language",
		`^nl\-NL$`,
		`en`,
		http.Header{"Accept-Language": []string{"nl-NL"}},
		"en",
		false,
	}, {
		"replace when header is provided and pattern matches anything",
		"Accept-Language",
		`^.*`,
		`en`,
		http.Header{"Accept-Language": []string{"nl-NL"}},
		"en",
		false,
	}, {
		"replace when header is not provided and pattern matches anything",
		"Accept-Language",
		`^.*`,
		`en`,
		http.Header{},
		"",
		true,
	}, {
		"do not replace when header is not provided and pattern does not match",
		"Accept-Language",
		`fr`,
		`en`,
		http.Header{},
		"",
		true,
	}} {
		t.Run(tt.msg, func(t *testing.T) {
			spec := NewModResponseHeader()
			f, err := spec.CreateFilter([]any{tt.headerName, tt.expression, tt.replacement})
			if err != nil {
				t.Error(err)
			}

			resp := http.Response{}
			resp.Header = http.Header{}

			maps.Copy(resp.Header, tt.responseHeader)

			ctx := &filtertest.Context{FResponse: &resp}
			f.Response(ctx)

			hv := resp.Header.Get(tt.headerName)
			if hv != tt.expectedHeader {
				t.Errorf(`failed to modify request header %s to "%s". Got: "%s"`, tt.headerName, tt.expectedHeader, hv)
			}

			if tt.expectHeaderToNotExist && len(resp.Header.Values(tt.headerName)) != 0 {
				t.Errorf(`expected header %s to not exist, but it does`, tt.headerName)
			}
		})
	}
}

func TestModifyHostWithInvalidExpression(t *testing.T) {
	spec := NewModRequestHeader()
	if f, err := spec.CreateFilter([]any{"Host", "(?=;)", "foo"}); err == nil || f != nil {
		t.Error("Expected error for invalid regular expression parameter")
	}
}

func testCreateHost(t *testing.T, spec func() filters.Spec, items []createTestItemHost) {
	for _, ti := range items {
		func() {
			f, err := spec().CreateFilter(ti.args)
			switch {
			case ti.err && err == nil:
				t.Error(ti.msg, "failed to fail")
			case !ti.err && err != nil:
				t.Error(ti.msg, err)
			case err == nil && f == nil:
				t.Error(ti.msg, "failed to create filter")
			}
		}()
	}
}

func TestCreateModHost(t *testing.T) {
	testCreateHost(t, NewModRequestHeader, []createTestItemHost{{
		"no args",
		nil,
		true,
	}, {
		"single arg",
		[]any{"Host"},
		true,
	}, {
		"two args",
		[]any{"Host", ".*"},
		true,
	}, {
		"non-string arg, pos 1",
		[]any{3.14, ".*", "/foo"},
		true,
	}, {
		"non-string arg, pos 2",
		[]any{"Host", 2.72, "/foo"},
		true,
	}, {
		"non-string arg, pos 3",
		[]any{"Host", ".*", 2.72},
		true,
	}, {
		"more than three args",
		[]any{"Host", ".*", "/foo", "/bar"},
		true,
	}, {
		"create",
		[]any{"Host", ".*", "/foo"},
		false,
	}})
}
