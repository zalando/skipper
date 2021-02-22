package script

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

type luaContext struct {
	request      *http.Request
	response     *http.Response
	pathParams   map[string]string
	bag          map[string]interface{}
	outgoingHost string
}

func (l *luaContext) ResponseWriter() http.ResponseWriter   { return nil }
func (l *luaContext) Request() *http.Request                { return l.request }
func (l *luaContext) Response() *http.Response              { return l.response }
func (l *luaContext) OriginalRequest() *http.Request        { return nil }
func (l *luaContext) OriginalResponse() *http.Response      { return nil }
func (l *luaContext) Served() bool                          { return false }
func (l *luaContext) MarkServed()                           {}
func (l *luaContext) Serve(_ *http.Response)                {}
func (l *luaContext) PathParam(n string) string             { return l.pathParams[n] }
func (l *luaContext) StateBag() map[string]interface{}      { return l.bag }
func (l *luaContext) BackendUrl() string                    { return "" }
func (l *luaContext) OutgoingHost() string                  { return l.outgoingHost }
func (l *luaContext) SetOutgoingHost(h string)              { l.outgoingHost = h }
func (l *luaContext) Metrics() filters.Metrics              { return nil }
func (l *luaContext) Tracer() opentracing.Tracer            { return nil }
func (l *luaContext) ParentSpan() opentracing.Span          { return nil }
func (l *luaContext) Split() (filters.FilterContext, error) { return nil, nil }
func (l *luaContext) Loopback()                             {}

type testContext struct {
	script         string
	params         []string
	pathParams     map[string]string
	url            string
	requestHeader  map[string]string
	responseHeader map[string]string
}

func TestScript(t *testing.T) {
	for _, test := range []struct {
		name                   string
		context                testContext
		expectedStateBag       map[string]string
		expectedRequestHeader  map[string]string
		expectedResponseHeader map[string]string
		expectedOutgoingHost   string
		expectedURL            string
		expectedStatus         int
	}{
		{
			name: "state bag",
			context: testContext{
				script: `function request(ctx, params); ctx.state_bag["foo"] = "bar"; end`,
			},
			expectedStateBag: map[string]string{"foo": "bar"},
		},
		{
			name: "get request header",
			context: testContext{
				script:        `function request(ctx, params); ctx.state_bag["User-Agent"] = ctx.request.header["User-Agent"]; end`,
				requestHeader: map[string]string{"user-agent": "luatest/1.0"},
			},
			expectedStateBag: map[string]string{"User-Agent": "luatest/1.0"},
		},
		{
			name: "set request header",
			context: testContext{
				script: `function request(ctx, params); ctx.request.header["User-Agent"] = "skipper.lua/1.0"; end`,
			},
			expectedRequestHeader: map[string]string{"user-agent": "skipper.lua/1.0"},
		},
		{
			name: "response header",
			context: testContext{
				script:         `function response(ctx, params); ctx.response.header["X-Baz"] = ctx.request.header["X-Foo"] .. ctx.response.header["X-Bar"]; end`,
				requestHeader:  map[string]string{"X-Foo": "Foo"},
				responseHeader: map[string]string{"X-Bar": "Bar"},
			},
			expectedResponseHeader: map[string]string{"X-Bar": "Bar", "X-Baz": "FooBar"},
		},
		{
			name: "outgoing host",
			context: testContext{
				script: `function response(ctx, params); ctx.request.outgoing_host = "qqq." .. ctx.request.outgoing_host; end`,
			},
			expectedOutgoingHost: "qqq.www.example.com",
		},
		{
			name: "set host request header",
			context: testContext{
				script: `set_request_header.lua`,
				params: []string{"Host", "new.example.com"},
			},
			expectedRequestHeader: map[string]string{"Host": "new.example.com"},
			expectedOutgoingHost:  "new.example.com",
		},
		{
			name: "mod path",
			context: testContext{
				script: `function request(ctx, params); ctx.request.url_path = "/beta" .. ctx.request.url_path; end`,
				url:    "http://www.example.com/foo/bar",
			},
			expectedURL: "http://www.example.com/beta/foo/bar"},
		{
			name: "set path without query",
			context: testContext{
				script: `set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com/foo/bar",
			},
			expectedURL: "http://www.example.com/new/path",
		},
		{
			name: "set path with query",
			context: testContext{
				script: `set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com/foo/bar?baz=1",
			},
			expectedURL: "http://www.example.com/new/path?baz=1",
		},
		{
			name: "set path empty path",
			context: testContext{
				script: `set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com",
			},
			expectedURL: "http://www.example.com/new/path",
		},
		{
			name: "set path empty path with query",
			context: testContext{
				script: `set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com?foo=bar",
			},
			expectedURL: "http://www.example.com/new/path?foo=bar",
		},
		{
			name: "response status",
			context: testContext{
				script: `function response(ctx, params); ctx.response.status_code = ctx.response.status_code + 1; end`,
			},
			expectedStatus: 201,
		},
		{
			name: "set query",
			context: testContext{
				script: `set_query.lua`,
				params: []string{"baz", "2"},
				url:    "http://www.example.com/foo/bar?baz=1&x=y",
			},
			expectedURL: "http://www.example.com/foo/bar?baz=2&x=y",
		},
		{
			name: "set query when empty",
			context: testContext{
				script: `set_query.lua`,
				params: []string{"baz", "2"},
				url:    "http://www.example.com/foo/bar",
			},
			expectedURL: "http://www.example.com/foo/bar?baz=2",
		},
		{
			name: "delete query",
			context: testContext{
				script: `set_query.lua`,
				params: []string{"baz", ""},
				url:    "http://www.example.com/foo/bar?baz=1",
			},
			expectedURL: "http://www.example.com/foo/bar",
		},
		{
			name: "strip query",
			context: testContext{
				script:        `strip_query.lua`,
				url:           "http://www.example.com/foo/bar?baz=1&x=y&z",
				requestHeader: map[string]string{"X-Dummy": "dummy"},
			},
			expectedURL:           "http://www.example.com/foo/bar",
			expectedRequestHeader: map[string]string{"X-Dummy": "dummy"},
		},
		{
			name: "strip query and preserve to headers",
			context: testContext{
				script: `strip_query.lua`,
				params: []string{"true"},
				url:    "http://www.example.com/foo/bar?baz=1&x=y&z",
			},
			expectedURL:           "http://www.example.com/foo/bar",
			expectedRequestHeader: map[string]string{"X-Query-Param-Baz": "1", "X-Query-Param-X": "y"},
		},
		{
			name: "print query",
			context: testContext{
				script: `function request(ctx, params)
					for k, v in ctx.request.url_query() do
						print(k, "=", v);
					end
				end`,
				url: "http://www.example.com/foo/bar?baz=1&x=y&z",
			},
			expectedStatus: 200, // not changed
		},
		{
			name: "set raw query",
			context: testContext{
				script: `function request(ctx, params); ctx.request.url_raw_query = "baz=2&x=y&z"; end`,
				url:    "http://www.example.com/foo/bar?baz=1",
			},
			expectedURL: "http://www.example.com/foo/bar?baz=2&x=y&z",
		},
		{
			name: "queryToHeader",
			context: testContext{
				script: `query_to_header.lua`,
				params: []string{"foo-query-param", "X-Foo-Header"},
				url:    "http://www.example.com/foo/bar?foo-query-param=test",
			},
			expectedRequestHeader: map[string]string{"X-Foo-Header": "test"},
		},
		{
			name: "queryToHeader when header is present",
			context: testContext{
				script:        `query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar?foo-query-param=test",
				requestHeader: map[string]string{"X-Foo-Header": "foo"},
			},
			expectedRequestHeader: map[string]string{"X-Foo-Header": "foo"},
		},
		{
			name: "queryToHeader when query is absent",
			context: testContext{
				script:        `query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar",
				requestHeader: map[string]string{"X-Dummy": "dummy"},
			},
			expectedRequestHeader: map[string]string{"X-Dummy": "dummy"},
		},
		{
			name: "queryToHeader when query is empty",
			context: testContext{
				script:        `query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar?foo-query-param=",
				requestHeader: map[string]string{"X-Dummy": "dummy"},
			},
			expectedRequestHeader: map[string]string{"X-Dummy": "dummy"},
		},
		{
			name: "path param to header",
			context: testContext{
				script:     `function request(ctx, params); ctx.request.header["X-Id"] = ctx.path_param["id"]; end`,
				pathParams: map[string]string{"id": "hello"},
			},
			expectedRequestHeader: map[string]string{"x-id": "hello"},
		},
		{
			name: "get request cookie",
			context: testContext{
				requestHeader: map[string]string{"Cookie": "PHPSESSID=298zf09hf012fh2; csrftoken=u32t4o3tb3gg43; _gat=1"},
				script: `function request(ctx, params)
					ctx.state_bag.PHPSESSID = ctx.request.cookie.PHPSESSID
					ctx.state_bag.csrftoken = ctx.request.cookie.csrftoken
					ctx.state_bag._gat = ctx.request.cookie._gat
					ctx.state_bag.absent = ctx.request.cookie.absent
				end`,
			},
			expectedStateBag: map[string]string{
				"PHPSESSID": "298zf09hf012fh2",
				"csrftoken": "u32t4o3tb3gg43",
				"_gat":      "1",
			},
		},
		{
			name: "iterate request cookies",
			context: testContext{
				requestHeader: map[string]string{"Cookie": "PHPSESSID=298zf09hf012fh2; csrftoken=u32t4o3tb3gg43; _gat=1; csrftoken=repeat"},
				script: `function request(ctx, params)
					ctx.state_bag.result = ""
					for n, v in ctx.request.cookie() do
						ctx.state_bag.result = ctx.state_bag.result .. n .. "=" .. v .. " "
					end
				end`,
			},
			expectedStateBag: map[string]string{
				"result": "PHPSESSID=298zf09hf012fh2 csrftoken=u32t4o3tb3gg43 _gat=1 csrftoken=repeat ",
			},
		},
	} {
		log.Println("Running", test.name, "test")

		fc, err := runFilter(&test.context)
		if err != nil {
			t.Errorf("failed to run filter: %v", err)
		}

		exp := len(test.expectedRequestHeader)
		if exp > 0 && exp != len(fc.Request().Header) {
			t.Errorf("[%s] request header mismatch: expected %v, got: %v", test.name, test.expectedRequestHeader, fc.Request().Header)
		}
		for k, v := range test.expectedRequestHeader {
			if fc.Request().Header.Get(k) != v {
				t.Errorf("[%s] %s request header: expected %s, got: %s", test.name, k, v, fc.Request().Header.Get(k))
			}
		}

		exp = len(test.expectedResponseHeader)
		if exp > 0 && exp != len(fc.Response().Header) {
			t.Errorf("[%s] response header mismatch: expected %v, got: %v", test.name, test.expectedResponseHeader, fc.Response().Header)
		}
		for k, v := range test.expectedResponseHeader {
			if fc.Response().Header.Get(k) != v {
				t.Errorf("[%s] %s response header: expected %s, got: %s", test.name, k, v, fc.Response().Header.Get(k))
			}
		}

		if len(fc.StateBag()) != len(test.expectedStateBag) {
			t.Errorf("[%s] state bag mismatch: expected %v, got: %v", test.name, test.expectedStateBag, fc.StateBag())
		}
		for k, v := range test.expectedStateBag {
			bv, ok := fc.StateBag()[k]
			if !ok || v != bv {
				t.Errorf("[%s] %s state bag: expected %s, got: %s", test.name, k, v, bv)
			}
		}

		if test.expectedOutgoingHost != "" && fc.OutgoingHost() != test.expectedOutgoingHost {
			t.Errorf("[%s] outgoing host: expected %s, got: %s", test.name, test.expectedOutgoingHost, fc.OutgoingHost())
		}
		if test.expectedURL != "" && fc.Request().URL.String() != test.expectedURL {
			t.Errorf("[%s] request path: expected %s, got: %v", test.name, test.expectedURL, fc.Request().URL)
		}
		if test.expectedStatus != 0 && test.expectedStatus != fc.Response().StatusCode {
			t.Errorf("[%s] response status: expected %d, got: %d", test.name, test.expectedStatus, fc.Response().StatusCode)
		}
	}
}

func TestSleep(t *testing.T) {
	ctx := &testContext{
		script: `function request(ctx, params) sleep(100.1) end`,
	}
	t0 := time.Now()
	_, err := runFilter(ctx)
	if err != nil {
		t.Fatalf("failed to run filter: %v", err)
	}
	t1 := time.Now()

	if t1.Sub(t0) < 100*time.Millisecond {
		t.Error("expected delay of 100 ms")
	}
}

// testable example have to refer known identifier
const LoadFileOK = `load_ok.lua`

func ExampleLoadFileOK() {
	runExample(&testContext{
		script: LoadFileOK,
	})
	// Output:
	// GET
}

const LoadOK = `
function request(ctx, params)
	print(ctx.request.method)
end`

func ExampleLoadOK() {
	runExample(&testContext{
		script: LoadOK,
	})
	// Output:
	// GET
}

const MissingFunc = `print("some string")`

func ExampleMissingFunc() {
	runExample(&testContext{
		script: MissingFunc,
	})
	// Output:
	// some string
	// at least one of `request` and `response` function must be present
}

const MalformedFile = `not_a_filter.lua`

func ExampleMalformedFile() {
	runExample(&testContext{
		script: MalformedFile,
	})
	// Output:
	// some string
	// at least one of `request` and `response` function must be present
}

const SyntaxError = `function request(ctx, params); print(ctx.request.method)`

func ExampleSyntaxError() {
	runExample(&testContext{
		script: SyntaxError,
	})
	// Output:
	// <script> at EOF:   syntax error
}

const PrintParams = `
function request(ctx, params)
	print(params[1])
	print(params[2])
	print(params[3])
	print(params[4])
	print(params.myparam)
	print(params.other)
	print(params.justkey)
	print(params.x)
end`

func ExamplePrintParams() {
	runExample(&testContext{
		script: PrintParams,
		params: []string{"myparam=foo", "other=bar", "justkey"},
	})
	// Output:
	// myparam=foo
	// other=bar
	// justkey
	// nil
	// foo
	// bar
	//
	// nil
}

const SetRequestInvalidField = `
function response(ctx, params)
	ctx.request.invalid_field = "test"
	print("ok")
end`

func ExampleSetRequestInvalidField() {
	runExample(&testContext{
		script: SetRequestInvalidField,
	})
	// Output:
	// ok
}

const GetRequestInvalidField = `
function response(ctx, params)
	print(ctx.request.invalid_field)
end`

func ExampleGetRequestInvalidField() {
	runExample(&testContext{
		script: GetRequestInvalidField,
	})
	// Output:
	// nil
}

const GetInvalidResponseStatus = `function response(ctx, params); ctx.response.status_code = "invalid"; end`

func ExampleGetInvalidResponseStatus() {
	runExample(&testContext{
		script: GetInvalidResponseStatus,
	})
	// Output:
	// Error calling response from function response(ctx, params); ctx.response.status_code = "invalid"; end: <script>:1: unsupported status_code type string, need a number
	// stack traceback:
	// 	[G]: in function (anonymous)
	// 	<script>:1: in main chunk
	// 	[G]: ?
}

const SetResponseInvalidField = `function response(ctx, params); ctx.response.invalid_field = "test"; end`

func ExampleSetResponseInvalidField() {
	runExample(&testContext{
		script: SetResponseInvalidField,
	})
	// Output:
	// Error calling response from function response(ctx, params); ctx.response.invalid_field = "test"; end: <script>:1: unsupported response field invalid_field
	// stack traceback:
	// 	[G]: in function (anonymous)
	// 	<script>:1: in main chunk
	// 	[G]: ?
}

const GetResponseInvalidField = `
function response(ctx, params)
	print(ctx.response.invalid_field)
end`

func ExampleGetResponseInvalidField() {
	runExample(&testContext{
		script: GetResponseInvalidField,
	})
	// Output:
	// nil
}

const LogHeaders = `log_header.lua`

func ExampleLogHeaders() {
	runExample(&testContext{
		script: LogHeaders,
		params: []string{"request", "response"},
		// single header as iteration order is not defined
		requestHeader:  map[string]string{"X-Foo": "foo"},
		responseHeader: map[string]string{"X-Bar": "bar"},
	})
	// Output:
	// GET http://www.example.com/foo/bar HTTP/1.1\r
	// Host: www.example.com\r
	// X-Foo: foo\r
	// \r
	//
	// Response for GET http://www.example.com/foo/bar HTTP/1.1\r
	// 200\r
	// X-Bar: bar\r
	// \r
}

const LogRequestAuthHeader = `log_header.lua`

func ExampleLogRequestAuthHeader() {
	runExample(&testContext{
		script: LogRequestAuthHeader,
		params: []string{"request"},
		// single header as iteration order is not defined
		requestHeader: map[string]string{"Authorization": "request secret"},
	})
	// Output:
	// GET http://www.example.com/foo/bar HTTP/1.1\r
	// Host: www.example.com\r
	// Authorization: TRUNCATED\r
	// \r
}

const PrintRequestUrlRawQuery = `
function request(ctx, params)
	print(ctx.request.url_raw_query)
end`

func ExamplePrintRequestUrlRawQuery() {
	runExample(&testContext{
		script: PrintRequestUrlRawQuery,
		url:    "http://www.example.com/foo/bar?baz=1&x=y&z",
	})
	// Output:
	// baz=1&x=y&z
}

const ServeWrongArgType = `
function request(ctx, params)
	ctx.serve("wrong")
	print("ok")
end`

func ExampleServeWrongArgType() {
	runExample(&testContext{
		script: ServeWrongArgType,
	})
	// Output:
	// ok
}

const ServeWithoutArgs = `
function request(ctx, params)
	ctx.serve()
	print("ok")
end`

func ExampleServeWithoutArgs() {
	runExample(&testContext{
		script: ServeWithoutArgs,
	})
	// Output:
	// ok
}

const ServeInvalidStatusCode = `
function request(ctx, params)
	ctx.serve({status_code="str"})
	print("ok")
end`

func ExampleServeInvalidStatusCode() {
	runExample(&testContext{
		script: ServeInvalidStatusCode,
	})
	// Output:
	// ok
}

const ServeInvalidHeader = `
function request(ctx, params)
	ctx.serve({header="str"})
	print("ok")
end`

func ExampleServeInvalidHeader() {
	runExample(&testContext{
		script: ServeInvalidHeader,
	})
	// Output:
	// ok
}

const SetMalformedRequestUrl = `
function request(ctx, params)
	ctx.request.url = ":foo"
	print(ctx.request.url)
end`

func ExampleSetMalformedRequestUrl() {
	runExample(&testContext{
		script: SetMalformedRequestUrl,
		url:    "http://www.example.com/foo/bar?baz=1",
	})
	// Output:
	// http://www.example.com/foo/bar?baz=1
}

const GetMissingRequestHeader = `
function request(ctx, params)
	print(ctx.request.header.missing)
end`

func ExampleGetMissingRequestHeader() {
	runExample(&testContext{
		script: GetMissingRequestHeader,
	})
	// Output:
	//
}

const GetMissingResponseHeader = `
function response(ctx, params)
	print(ctx.response.header.missing)
end`

func ExampleGetMissingResponseHeader() {
	runExample(&testContext{
		script: GetMissingResponseHeader,
	})
	// Output:
	//
}

const GetMissingRequestUrlQuery = `
function request(ctx, params)
	print(ctx.request.url_query.missing)
end`

func ExampleGetMissingRequestUrlQuery() {
	runExample(&testContext{
		script: GetMissingRequestUrlQuery,
		url:    "http://www.example.com/foo/bar?baz=1",
	})
	// Output:
	// nil
}

const GetMissingPathParam = `
function request(ctx, params)
	print(ctx.path_param.missing)
end`

func ExampleGetMissingPathParam() {
	runExample(&testContext{
		script: GetMissingPathParam,
	})
	// Output:
	// nil
}

const GetMissingStateBag = `
function request(ctx, params)
	print(ctx.state_bag.missing)
end`

func ExampleGetMissingStateBag() {
	runExample(&testContext{
		script: GetMissingStateBag,
	})
	// Output:
	// nil
}

const SetUnsupportedStateBag = `
function request(ctx, params)
	ctx.state_bag.unsupported = {}
	print("ok")
end`

func ExampleSetUnsupportedStateBag() {
	runExample(&testContext{
		script: SetUnsupportedStateBag,
	})
	// Output:
	// ok
}

func runFilter(test *testContext) (*luaContext, error) {
	ls := &luaScript{}
	args := []interface{}{test.script}
	for _, p := range test.params {
		args = append(args, p)
	}
	scr, err := ls.CreateFilter(args)
	if err != nil {
		return nil, err
	}
	url := "http://www.example.com/foo/bar"
	if test.url != "" {
		url = test.url
	}
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range test.requestHeader {
		req.Header.Add(k, v)
	}
	fc := &luaContext{
		pathParams:   test.pathParams,
		bag:          make(map[string]interface{}),
		request:      req,
		outgoingHost: "www.example.com",
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
		Header:        make(http.Header),
	}
	for k, v := range test.responseHeader {
		fc.Response().Header.Add(k, v)
	}

	scr.Response(fc)

	return fc, nil
}

func runExample(ctx *testContext) {
	o := log.StandardLogger().Out
	f := log.StandardLogger().Formatter
	defer func() {
		log.SetOutput(o)
		log.SetFormatter(f)
	}()

	log.SetOutput(os.Stdout)
	log.SetFormatter(&exampleLogFormatter{})

	_, err := runFilter(ctx)
	if err != nil {
		log.Errorf("%v", err)
	}
}

type exampleLogFormatter struct {
}

func (f *exampleLogFormatter) Format(entry *log.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}
	// escape \r to use testable examples
	b.WriteString(strings.ReplaceAll(entry.Message, "\r", `\r`))
	b.WriteByte('\n')
	return b.Bytes(), nil
}

const SetRequestCookieIsNotSupported = `function request(ctx, params); ctx.request.cookie["test"] = "test"; end`

func ExampleSetRequestCookieIsNotSupported() {
	runExample(&testContext{
		script: SetRequestCookieIsNotSupported,
	})
	// Output:
	// Error calling request from function request(ctx, params); ctx.request.cookie["test"] = "test"; end: <script>:1: setting cookie is not supported
	// stack traceback:
	// 	[G]: in function (anonymous)
	// 	<script>:1: in main chunk
	// 	[G]: ?
}
