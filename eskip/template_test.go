package eskip

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

type createTestItem struct {
	template string
	expected string
	getter   TemplateGetter
}

func testCreate(t *testing.T, items []createTestItem) {
	for _, ti := range items {
		func() {
			template := NewTemplate(ti.template)
			result := template.Apply(ti.getter)
			if result != ti.expected {
				t.Errorf("Error: '%s' != '%s'", result, ti.expected)
			}
		}()
	}
}

func TestTemplateGetter(t *testing.T) {
	testCreate(t, []createTestItem{{
		"template",
		"template",
		func(param string) string {
			return param
		},
	}, {
		"/path/${param1}/",
		"/path/param1/",
		func(param string) string {
			return param
		},
	}, {
		"/${param2}/${param1}/",
		"/param2/param1/",
		func(param string) string {
			return param
		},
	}, {
		"/${param2}",
		"/param2",
		func(param string) string {
			return param
		},
	}, {
		"/${missing}",
		"/",
		func(param string) string {
			return ""
		},
	}, {
		"/${param1}",
		"/${param1}",
		nil,
	}})
}

func TestTemplateApplyRequestResponseContext(t *testing.T) {
	parseUrl := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	for _, ti := range []struct {
		name           string
		template       string
		context        *filtertest.Context
		requestExpect  string
		requestOk      bool
		responseExpect string
		responseOk     bool
	}{{
		"path params",
		"hello ${p1} ${p2}",
		&filtertest.Context{
			FParams: map[string]string{
				"p1": "path",
				"p2": "params",
			},
		},
		"hello path params",
		true,
		"hello path params",
		true,
	}, {
		"all missing",
		"hello ${p1} ${p2}",
		&filtertest.Context{},
		"hello  ",
		false,
		"hello  ",
		false,
	}, {
		"some missing",
		"hello ${p1} ${missing}",
		&filtertest.Context{
			FParams: map[string]string{
				"p1": "X",
			},
		},
		"hello X ",
		false,
		"hello X ",
		false,
	}, {
		"request header",
		"hello ${request.header.X-Foo}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"X-Foo": []string{"foo"},
				},
			},
		},
		"hello foo",
		true,
		"hello foo",
		true,
	}, {
		"missing request header",
		"hello ${request.header.X-Foo}",
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"hello ",
		false,
		"hello ",
		false,
	}, {
		"response header",
		"hello ${response.header.X-Foo}",
		&filtertest.Context{
			FResponse: &http.Response{
				Header: http.Header{
					"X-Foo": []string{"foo"},
				},
			},
		},
		"hello ",
		false, // response headers are not available in request context
		"hello foo",
		true,
	}, {
		"missing response header",
		"hello ${response.header.X-Foo}",
		&filtertest.Context{
			FResponse: &http.Response{},
		},
		"hello ",
		false,
		"hello ",
		false,
	}, {
		"request and response headers (lowercase)",
		"hello ${request.header.x-foo} ${response.header.x-foo}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"X-Foo": []string{"bar"},
				},
			},
			FResponse: &http.Response{
				Header: http.Header{
					"X-Foo": []string{"baz"},
				},
			},
		},
		"hello bar ",
		false, // response headers are not available in request context
		"hello bar baz",
		true,
	}, {
		"request query",
		"hello ${request.query.q-Q} ${request.query.P_p}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/path?q-Q=foo&P_p=bar"),
			},
		},
		"hello foo bar",
		true,
		"hello foo bar",
		true,
	}, {
		"missing request query",
		"hello ${request.query.missing}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/path?p=foo"),
			},
		},
		"hello ",
		false,
		"hello ",
		false,
	}, {
		"request cookie",
		"hello ${request.cookie.foo} ${request.cookie.x}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"Cookie": []string{"foo=bar; x=y"},
				},
			},
		},
		"hello bar y",
		true,
		"hello bar y",
		true,
	}, {
		"missing request cookie",
		"hello ${request.cookie.missing}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"Cookie": []string{"foo=bar; x=y"},
				},
			},
		},
		"hello ",
		false,
		"hello ",
		false,
	}, {
		"request path",
		"hello ${request.path}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/foo/bar"),
			},
		},
		"hello /foo/bar",
		true,
		"hello /foo/bar",
		true,
	}, {
		"request path is empty",
		"hello ${request.path}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com"),
			},
		},
		"hello ",
		false, // path is empty
		"hello ",
		false,
	}, {
		"all in one",
		"Hello ${name} ${request.header.name} ${request.query.name} ${request.cookie.name} ${response.header.name}",
		&filtertest.Context{
			FParams: map[string]string{
				"name": "one",
			},
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/path?name=three"),
				Header: http.Header{
					"Name":   []string{"two"},
					"Cookie": []string{"name=four"},
				},
			},
			FResponse: &http.Response{
				Header: http.Header{
					"Name": []string{"five"},
				},
			},
		},
		"Hello one two three four ",
		false, // response headers are not available in request context
		"Hello one two three four five",
		true,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			template := NewTemplate(ti.template)
			result, ok := template.ApplyRequestContext(ti.context)
			if result != ti.requestExpect || ok != ti.requestOk {
				t.Errorf("Apply request context result mismatch: '%s' (%v) != '%s' (%v)", result, ok, ti.requestExpect, ti.requestOk)
			}

			result, ok = template.ApplyResponseContext(ti.context)
			if result != ti.responseExpect || ok != ti.responseOk {
				t.Errorf("Apply response context result mismatch: '%s' (%v) != '%s' (%v)", result, ok, ti.responseExpect, ti.responseOk)
			}
		})
	}
}
