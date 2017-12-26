package script

import (
	"testing"
        opentracing "github.com/opentracing/opentracing-go"
	"net/http"
	"github.com/zalando/skipper/filters"
)

func TestLoadScript(t *testing.T) {
	for _, test := range []struct{
		name	string
		code    string
		returnsOK	bool
	}{
		{
			"load_ok",
			`function request(ctx, params); print(ctx.request.method); end`,
			true,
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
	request *http.Request
	response *http.Response
	bag	map[string]interface{}
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

func (l *luaContext) MarkServed() { }

func (l *luaContext) Serve(_ *http.Response) { }

func (l *luaContext) PathParam(_ string) string { return "" }

func (l *luaContext) StateBag() map[string]interface{} {
	return l.bag
}

func (l *luaContext) BackendUrl() string { return "" }

func (l *luaContext) OutgoingHost() string { return "www.example.com" }

func (l *luaContext) SetOutgoingHost(_ string) { }

func (l *luaContext) Metrics() filters.Metrics { return nil }

func (l *luaContext) Tracer() opentracing.Tracer { return nil }

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
