package script

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

func TestLoadScript(t *testing.T) {
	for _, test := range []struct {
		name      string
		code      string
		returnsOK bool
	}{
		{
			"file load ok",
			`load_ok.lua`,
			true,
		},
		{
			"load ok",
			`function request(ctx, params); print(ctx.request.method); end`,
			true,
		},
		{
			"malformed file",
			`not_a_filter.lua`,
			false,
		},
		{
			"missing func",
			`print("some string")`,
			false,
		},
		{
			"syntax error",
			`function request(ctx, params); print(ctx.request.method)`,
			false,
		},
	} {
		ls := &luaScript{}
		_, err := ls.CreateFilter([]interface{}{test.code, "foo=bar"})
		if (err == nil) != test.returnsOK {
			t.Errorf("test %s returns unexpected error value: %s", test.name, err)
		}
	}
}

type luaContext struct {
	request  *http.Request
	response *http.Response
	bag      map[string]interface{}
}

func (l *luaContext) ResponseWriter() http.ResponseWriter {
	return nil
}

func (l *luaContext) Request() *http.Request {
	return l.request
}

func (l *luaContext) Response() *http.Response {
	return l.response
}

func (l *luaContext) OriginalRequest() *http.Request {
	return nil
}

func (l *luaContext) OriginalResponse() *http.Response {
	return nil
}

func (l *luaContext) Served() bool {
	return false
}

func (l *luaContext) MarkServed() {}

func (l *luaContext) Serve(_ *http.Response) {}

func (l *luaContext) PathParam(_ string) string { return "" }

func (l *luaContext) StateBag() map[string]interface{} {
	return l.bag
}

func (l *luaContext) BackendUrl() string { return "" }

func (l *luaContext) OutgoingHost() string { return "www.example.com" }

func (l *luaContext) SetOutgoingHost(_ string) {}

func (l *luaContext) Metrics() filters.Metrics { return nil }

func (l *luaContext) Tracer() opentracing.Tracer { return nil }

func (l *luaContext) ParentSpan() opentracing.Span { return nil }

func (l *luaContext) Split() (filters.FilterContext, error) { return nil, nil }

func (l *luaContext) Loopback() {}

func TestStateBag(t *testing.T) {
	code := `function request(ctx, params); ctx.state_bag["foo"] = "bar"; end`
	ls := &luaScript{}
	scr, err := ls.CreateFilter([]interface{}{code})
	if err != nil {
		t.Errorf("failed to compile test code: %s", err)
	}
	fc := &luaContext{bag: make(map[string]interface{})}
	scr.Request(fc)
	if fc.StateBag()["foo"].(string) != "bar" {
		t.Errorf("failed to set statebag value")
	}
}

func TestGetRequestHeader(t *testing.T) {
	code := `function request(ctx, params); ctx.state_bag["User-Agent"] = ctx.request.header["User-Agent"]; end`
	ls := &luaScript{}
	scr, err := ls.CreateFilter([]interface{}{code})
	if err != nil {
		t.Errorf("failed to compile test code: %s", err)
	}
	req, _ := http.NewRequest("GET", "http://www.example.com/", nil)
	req.Header.Set("user-agent", "luatest/1.0")
	fc := &luaContext{
		bag:     make(map[string]interface{}),
		request: req,
	}
	scr.Request(fc)
	if fc.StateBag()["User-Agent"].(string) != "luatest/1.0" {
		t.Errorf("failed to get request header value")
	}
}

func TestSetRequestHeader(t *testing.T) {
	code := `function request(ctx, params); ctx.request.header["User-Agent"] = "skipper.lua/1.0"; end`
	ls := &luaScript{}
	scr, err := ls.CreateFilter([]interface{}{code})
	if err != nil {
		t.Errorf("failed to compile test code: %s", err)
	}
	req, _ := http.NewRequest("GET", "http://www.example.com/", nil)
	fc := &luaContext{
		bag:     make(map[string]interface{}),
		request: req,
	}
	scr.Request(fc)
	if fc.request.Header.Get("User-Agent") != "skipper.lua/1.0" {
		t.Errorf("failed to set request header value")
	}
}

func TestResponseHeader(t *testing.T) {
	code := `
	function response(ctx, params)
		ctx.response.header["X-Baz"] = ctx.request.header["X-Foo"] .. ctx.response.header["X-Bar"];
	end`
	ls := &luaScript{}
	scr, err := ls.CreateFilter([]interface{}{code})
	if err != nil {
		t.Errorf("failed to compile test code: %s", err)
	}
	req, _ := http.NewRequest("GET", "http://www.example.com/foo/bar", nil)
	req.Header.Add("X-Foo", "Foo")
	fc := &luaContext{
		bag:     make(map[string]interface{}),
		request: req,
	}
	scr.Request(fc)

	body := "Hello world"
	fc.response = &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Header:        make(http.Header, 0),
	}
	fc.response.Header.Add("X-Bar", "Bar")

	scr.Response(fc)

	if fc.response.Header.Get("X-Baz") != "FooBar" {
		t.Errorf("failed to set response header, got: %v", fc.response.Header.Get("X-Baz"))
	}
}
