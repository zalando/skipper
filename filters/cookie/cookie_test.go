package cookie

import (
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestCreateFilter(t *testing.T) {
	for _, ti := range []struct {
		msg   string
		typ   direction
		args  []interface{}
		check filter
		err   bool
	}{{
		"too few arguments",
		response,
		[]interface{}{"test-cookie"},
		filter{},
		true,
	}, {
		"too many arguments",
		response,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg, "something"},
		filter{},
		true,
	}, {
		"too few arguments, js",
		responseJS,
		[]interface{}{"test-cookie"},
		filter{},
		true,
	}, {
		"too many arguments, js",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg, "something"},
		filter{},
		true,
	}, {
		"too many arguments for request cookie",
		request,
		[]interface{}{"test-cookie", "A", 42.0},
		filter{},
		true,
	}, {
		"wrong name type",
		response,
		[]interface{}{3.14, "A", 42.0},
		filter{},
		true,
	}, {
		"empty name",
		response,
		[]interface{}{"", "A", 42.0},
		filter{},
		true,
	}, {
		"wrong value type",
		response,
		[]interface{}{"test-cookie", 3.14, 42.0},
		filter{},
		true,
	}, {
		"wrong ttl type",
		response,
		[]interface{}{"test-cookie", "A", "42"},
		filter{},
		true,
	}, {
		"wrong name type, JS",
		responseJS,
		[]interface{}{3.14, "A", 42.0},
		filter{},
		true,
	}, {
		"empty name, JS",
		responseJS,
		[]interface{}{"", "A", 42.0},
		filter{},
		true,
	}, {
		"wrong value type, JS",
		responseJS,
		[]interface{}{"test-cookie", 3.14, 42.0},
		filter{},
		true,
	}, {
		"wrong ttl type, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", "42"},
		filter{},
		true,
	}, {
		"request cookie",
		request,
		[]interface{}{"test-cookie", "A"},
		filter{typ: request, name: "test-cookie", value: "A"},
		false,
	}, {
		"response session cookie",
		response,
		[]interface{}{"test-cookie", "A"},
		filter{typ: response, name: "test-cookie", value: "A"},
		false,
	}, {
		"response persistent cookie",
		response,
		[]interface{}{"test-cookie", "A", 42.0},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second},
		false,
	}, {
		"response persistent cookie, not change only, explicit",
		response,
		[]interface{}{"test-cookie", "A", 42.0, "always"},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second},
		false,
	}, {
		"response persistent cookie, change only",
		response,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second, changeOnly: true},
		false,
	}, {
		"response session cookie, JS",
		responseJS,
		[]interface{}{"test-cookie", "A"},
		filter{typ: response, name: "test-cookie", value: "A"},
		false,
	}, {
		"response persistent cookie, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second},
		false,
	}, {
		"response persistent cookie, not change only, explicit, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, "always"},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second},
		false,
	}, {
		"response persistent cookie, change only, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		filter{typ: response, name: "test-cookie", value: "A", ttl: 42 * time.Second, changeOnly: true},
		false,
	}} {
		var s filters.Spec
		switch ti.typ {
		case request:
			s = NewRequestCookie()
		case response:
			s = NewResponseCookie()
		case responseJS:
			s = NewJSCookie()
		}

		f, err := s.CreateFilter(ti.args)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
		} else if !ti.err {
			ff := f.(*filter)

			if ff.typ != ti.typ {
				t.Error(ti.msg, "direction", ff.typ, ti.typ)
			}

			if ff.name != ti.check.name {
				t.Error(ti.msg, "name", ff.name, ti.check.name)
			}

			if ff.value != ti.check.value {
				t.Error(ti.msg, "value", ff.value, ti.check.value)
			}

			if ff.ttl != ti.check.ttl {
				t.Error(ti.msg, "ttl", ff.ttl, ti.check.ttl)
			}
		}
	}
}

func TestSetCookie(t *testing.T) {
	const (
		domain = "example.org"
		host   = "www.example.org:80"
	)

	for _, ti := range []struct {
		msg           string
		typ           direction
		args          []interface{}
		requestCookie string
		check         *http.Cookie
	}{{
		"request cookie",
		request,
		[]interface{}{"test-cookie", "A"},
		"",
		&http.Cookie{Name: "test-cookie", Value: "A"},
	}, {
		"response cookie",
		response,
		[]interface{}{"test-cookie", "A"},
		"",
		&http.Cookie{
			Name:     "test-cookie",
			Value:    "A",
			HttpOnly: true,
			Domain:   domain,
			Path:     "/"},
	}, {
		"response cookie, with ttl",
		response,
		[]interface{}{"test-cookie", "A", 42.0},
		"",
		&http.Cookie{
			Name:     "test-cookie",
			Value:    "A",
			HttpOnly: true,
			Domain:   domain,
			Path:     "/",
			MaxAge:   42},
	}, {
		"response cookie, with non-sliding ttl",
		response,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"",
		&http.Cookie{
			Name:     "test-cookie",
			Value:    "A",
			HttpOnly: true,
			Domain:   domain,
			Path:     "/",
			MaxAge:   42},
	}, {
		"response cookie, with non-sliding ttl, request contains different value",
		response,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"B",
		&http.Cookie{
			Name:     "test-cookie",
			Value:    "A",
			HttpOnly: true,
			Domain:   domain,
			Path:     "/",
			MaxAge:   42},
	}, {
		"response cookie, with non-sliding ttl, request contains the same value",
		response,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"A",
		nil,
	}, {
		"response cookie, JS",
		responseJS,
		[]interface{}{"test-cookie", "A"},
		"",
		&http.Cookie{
			Name:   "test-cookie",
			Value:  "A",
			Domain: domain,
			Path:   "/"},
	}, {
		"response cookie, with ttl, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0},
		"",
		&http.Cookie{
			Name:   "test-cookie",
			Value:  "A",
			Domain: domain,
			Path:   "/",
			MaxAge: 42},
	}, {
		"response cookie, with non-sliding ttl, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"",
		&http.Cookie{
			Name:   "test-cookie",
			Value:  "A",
			Domain: domain,
			Path:   "/",
			MaxAge: 42},
	}, {
		"response cookie, with non-sliding ttl, request contains different value, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"B",
		&http.Cookie{
			Name:   "test-cookie",
			Value:  "A",
			Domain: domain,
			Path:   "/",
			MaxAge: 42},
	}, {
		"response cookie, with non-sliding ttl, request contains the same value, JS",
		responseJS,
		[]interface{}{"test-cookie", "A", 42.0, ChangeOnlyArg},
		"A",
		nil,
	}} {
		var s filters.Spec
		switch ti.typ {
		case request:
			s = NewRequestCookie()
		case response:
			s = NewResponseCookie()
		case responseJS:
			s = NewJSCookie()
		}

		f, err := s.CreateFilter(ti.args)
		if err != nil {
			t.Error(err)
			continue
		}

		ctx := &filtertest.Context{
			FRequest: &http.Request{
				Header: http.Header{},
				Host:   host},
			FStateBag: map[string]interface{}{},
			FResponse: &http.Response{Header: http.Header{}}}
		if ti.requestCookie != "" {
			ctx.Request().AddCookie(&http.Cookie{
				Name:  ti.args[0].(string),
				Value: ti.requestCookie})
		}

		if ti.typ == request {
			f.Request(ctx)
			if _, err := ctx.Request().Cookie(ti.check.Name); err != nil {
				t.Error(ti.msg, "request cookie")
			}
		} else {
			f.Response(ctx)
			if ti.check == nil {
				if len(ctx.Response().Cookies()) > 0 {
					t.Error(ti.msg, "cookie should not have been set")
				}

				continue
			}

			var c *http.Cookie
			for _, ci := range ctx.Response().Cookies() {
				if ci.Name == ti.check.Name {
					c = ci
					break
				}
			}

			if c == nil {
				t.Error(ti.msg, "missing cookie")
				continue
			}

			if c.Value != ti.check.Value {
				t.Error(ti.msg, "value", c.Value, ti.check.Value)
			}

			if c.HttpOnly != ti.check.HttpOnly {
				t.Error(ti.msg, "HttpOnly", c.HttpOnly, ti.check.HttpOnly)
			}

			if c.Domain != ti.check.Domain {
				t.Error(ti.msg, "domain", c.Domain, ti.check.Domain)
			}

			if c.Path != ti.check.Path {
				t.Error(ti.msg, "path", c.Path, ti.check.Path)
			}

			if c.MaxAge != ti.check.MaxAge {
				t.Error(ti.msg, "max-age", c.MaxAge, ti.check.MaxAge)
			}
		}
	}
}

func TestDropCookie(t *testing.T) {
	for _, tt := range []struct {
		name        string
		arg         string
		cookies     http.Header
		wantCookies map[string]string
		wantErr     bool
	}{
		{
			name:        "test no cookies",
			arg:         "no-cookie",
			cookies:     nil,
			wantCookies: nil,
			wantErr:     false,
		},
		{
			name: "test one cookie not match",
			arg:  "no-match",
			cookies: http.Header{
				"Cookie": []string{
					"foo=foo1",
				},
			},
			wantCookies: map[string]string{"foo": "foo1"},
			wantErr:     false,
		},
		{
			name: "test one cookie with match",
			arg:  "foo",
			cookies: http.Header{
				"Cookie": []string{
					"foo=foo1",
				},
			},
			wantCookies: nil,
			wantErr:     false,
		},
		{
			name: "test two cookies and one cookie with match",
			arg:  "foo",
			cookies: http.Header{
				"Cookie": []string{
					"foo=foo1",
					"bar=baz",
				},
			},
			wantCookies: map[string]string{"bar": "baz"},
			wantErr:     false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewDropRequestCookie()
			f, err := spec.CreateFilter([]interface{}{tt.arg})
			if err != nil {
				t.Fatalf("Failed to create filter: %v", err)
			}

			if f == nil {
				t.Fatal("Failed to create filter: filter nil")
			}

			ctx := &filtertest.Context{
				FRequest: &http.Request{
					Header: tt.cookies,
					Host:   "foo"},
				FStateBag: map[string]interface{}{},
				FResponse: &http.Response{Header: http.Header{}},
			}

			f.Request(ctx)

			if c, err := ctx.Request().Cookie(tt.arg); err != http.ErrNoCookie {
				t.Fatalf("Failed to delete cookie %s: %q", tt.arg, c)
			}
			if len(tt.wantCookies) != len(ctx.Request().Cookies()) {
				t.Fatalf("Failed to get right len of cookies %d != %d", len(tt.wantCookies), len(ctx.Request().Cookies()))
			}

			for k, v := range tt.wantCookies {
				cookie, err := ctx.Request().Cookie(k)
				if err != nil {
					t.Fatalf("Failed to get cookie %q: %v", k, err)
				}

				if cookie.Value != v {
					t.Fatalf("Failed to get cookie value %q, got: %q", v, cookie.Value)
				}
			}

			for _, cookie := range ctx.Request().Cookies() {
				if _, ok := tt.wantCookies[cookie.Name]; !ok {
					t.Fatalf("Failed to delete cookie: %s", cookie.Name)
				}
			}
		})
	}

}
