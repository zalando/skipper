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

func TestTemplateApplyContext(t *testing.T) {
	parseUrl := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	request := func(method, url string) *http.Request {
		r, err := http.NewRequest(method, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	for _, ti := range []struct {
		name     string
		template string
		context  *filtertest.Context
		expected string
		ok       bool
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
	}, {
		"all missing",
		"hello ${p1} ${p2}",
		&filtertest.Context{},
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
	}, {
		"missing request header",
		"hello ${request.header.X-Foo}",
		&filtertest.Context{
			FRequest: &http.Request{},
		},
		"hello ",
		false,
	}, {
		"response header",
		"hello ${response.header.X-Foo}",
		&filtertest.Context{
			FResponse: &http.Response{
				Header: http.Header{
					"X-Foo": []string{"bar"},
				},
			},
		},
		"hello bar",
		true,
	}, {
		"response header when response is absent",
		"hello ${response.header.X-Foo}",
		&filtertest.Context{},
		"hello ",
		false,
	}, {
		"missing response header",
		"hello ${response.header.X-Foo}",
		&filtertest.Context{
			FResponse: &http.Response{},
		},
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
		"hello bar baz",
		true,
	}, {
		"request and response headers when response is absent",
		"hello ${request.header.x-foo} ${response.header.x-foo}",
		&filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{
					"X-Foo": []string{"bar"},
				},
			},
		},
		"hello bar ",
		false,
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

	}, {
		"request rawquery",
		"hello ${request.rawQuery}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/path?q-Q=foo&P_p=bar&r=baz%20qux&s"),
			},
		},
		"hello q-Q=foo&P_p=bar&r=baz%20qux&s",
		true,
	}, {
		"request rawquery is empty",
		"hello ${request.rawQuery}",
		&filtertest.Context{
			FRequest: &http.Request{
				URL: parseUrl("http://example.com/path"),
			},
		},
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
		"Hello one two three four five",
		true,
	}, {
		"request source and X-Forwarded-For present",
		"${request.source}",
		&filtertest.Context{
			FRequest: &http.Request{
				RemoteAddr: "192.168.0.1:9876",
				Header: http.Header{
					"X-Forwarded-For": []string{"203.0.113.195, 70.41.3.18, 150.172.238.178"},
				},
			},
		},
		"203.0.113.195",
		true,
	}, {
		"request source and X-Forwarded-For absent",
		"${request.source}",
		&filtertest.Context{
			FRequest: &http.Request{
				RemoteAddr: "192.168.0.1:9876",
			},
		},
		"192.168.0.1",
		true,
	}, {
		"request sourceFromLast and X-Forwarded-For present",
		"${request.sourceFromLast}",
		&filtertest.Context{
			FRequest: &http.Request{
				RemoteAddr: "192.168.0.1:9876",
				Header: http.Header{
					"X-Forwarded-For": []string{"203.0.113.195, 70.41.3.18, 150.172.238.178"},
				},
			},
		},
		"150.172.238.178",
		true,
	}, {
		"request sourceFromLast and X-Forwarded-For absent",
		"${request.sourceFromLast}",
		&filtertest.Context{
			FRequest: &http.Request{
				RemoteAddr: "192.168.0.1:9876",
			},
		},
		"192.168.0.1",
		true,
	}, {
		"request clientIP (ignores X-Forwarded-For)",
		"${request.clientIP}",
		&filtertest.Context{
			FRequest: &http.Request{
				RemoteAddr: "192.168.0.1:9876",
				Header: http.Header{
					"X-Forwarded-For": []string{"203.0.113.195, 70.41.3.18, 150.172.238.178"},
				},
			},
		},
		"192.168.0.1",
		true,
	}, {
		"request method host",
		"${request.method} ${request.host}",
		&filtertest.Context{
			FRequest: request("GET", "https://example.com/test/1"),
		},
		"GET example.com",
		true,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			template := NewTemplate(ti.template)
			result, ok := template.ApplyContext(ti.context)
			if result != ti.expected || ok != ti.ok {
				t.Errorf("Apply context result mismatch: '%s' (%v) != '%s' (%v)", result, ok, ti.expected, ti.ok)
			}
		})
	}
}
