package script

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

type testContext struct {
	script         string
	params         []string
	pathParams     map[string]string
	url            string
	requestHeader  http.Header
	responseHeader http.Header
}

func testScript(script string) *testContext {
	return &testContext{script: script}
}

func TestScript(t *testing.T) {
	for _, test := range []struct {
		name                   string
		opts                   LuaOptions
		context                testContext
		expectedStateBag       map[string]string
		expectedRequestHeader  http.Header
		expectedResponseHeader http.Header
		expectedOutgoingHost   string
		expectedURL            string
		expectedStatus         int
		expectedError          error
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
				requestHeader: http.Header{"User-Agent": []string{"luatest/1.0"}},
			},
			expectedStateBag: map[string]string{"User-Agent": "luatest/1.0"},
		},
		{
			name: "set request header",
			context: testContext{
				script: `function request(ctx, params); ctx.request.header["User-Agent"] = "skipper.lua/1.0"; end`,
			},
			expectedRequestHeader: http.Header{"User-Agent": []string{"skipper.lua/1.0"}},
		},
		{
			name: "add request header",
			context: testContext{
				script: `function request(ctx, params)
					ctx.request.header.add("Foo", "Bar")
					ctx.request.header.add("Foo", "Baz")
				end`,
			},
			expectedRequestHeader: http.Header{"Foo": []string{"Bar", "Baz"}},
		},
		{
			name: "request header values",
			context: testContext{
				script: `function request(ctx, params)
					ctx.request.header.add("Foo", "Bar")
					ctx.request.header.add("Foo", "Baz")

					ctx.request.header["Qux"] = table.concat(ctx.request.header.values("Foo"), ", ")
					ctx.request.header["Absent-Length"] = table.getn(ctx.request.header.values("Absent"))
				end`,
			},
			expectedRequestHeader: http.Header{
				"Foo":           []string{"Bar", "Baz"},
				"Qux":           []string{"Bar, Baz"},
				"Absent-Length": []string{"0"},
			},
		},
		{
			name: "response header",
			context: testContext{
				script:         `function response(ctx, params); ctx.response.header["X-Baz"] = ctx.request.header["X-Foo"] .. ctx.response.header["X-Bar"]; end`,
				requestHeader:  http.Header{"X-Foo": []string{"Foo"}},
				responseHeader: http.Header{"X-Bar": []string{"Bar"}},
			},
			expectedResponseHeader: http.Header{"X-Bar": []string{"Bar"}, "X-Baz": []string{"FooBar"}},
		},
		{
			name: "add response header",
			context: testContext{
				script: `function response(ctx, params)
					ctx.response.header.add("X-Baz", ctx.request.header["X-Foo"])
					ctx.response.header.add("X-Baz", ctx.response.header["X-Bar"])
				end`,
				requestHeader:  http.Header{"X-Foo": []string{"Foo"}},
				responseHeader: http.Header{"X-Bar": []string{"Bar"}},
			},
			expectedResponseHeader: http.Header{
				"X-Bar": []string{"Bar"},
				"X-Baz": []string{"Foo", "Bar"},
			},
		},
		{
			name: "response header values",
			context: testContext{
				script: `function response(ctx, params)
					ctx.response.header.add("Foo", "Bar")
					ctx.response.header.add("Foo", "Baz")

					ctx.response.header["Qux"] = table.concat(ctx.response.header.values("Foo"), ", ")
					ctx.response.header["Absent-Length"] = table.getn(ctx.response.header.values("Absent"))
				end`,
			},
			expectedResponseHeader: http.Header{
				"Foo":           []string{"Bar", "Baz"},
				"Qux":           []string{"Bar, Baz"},
				"Absent-Length": []string{"0"},
			},
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
				script: `testdata/set_request_header.lua`,
				params: []string{"Host", "new.example.com"},
			},
			expectedRequestHeader: http.Header{"Host": []string{"new.example.com"}},
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
				script: `testdata/set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com/foo/bar",
			},
			expectedURL: "http://www.example.com/new/path",
		},
		{
			name: "set path with query",
			context: testContext{
				script: `testdata/set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com/foo/bar?baz=1",
			},
			expectedURL: "http://www.example.com/new/path?baz=1",
		},
		{
			name: "set path empty path",
			context: testContext{
				script: `testdata/set_path.lua`,
				params: []string{"/new/path"},
				url:    "http://www.example.com",
			},
			expectedURL: "http://www.example.com/new/path",
		},
		{
			name: "set path empty path with query",
			context: testContext{
				script: `testdata/set_path.lua`,
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
				script: `testdata/set_query.lua`,
				params: []string{"baz", "2"},
				url:    "http://www.example.com/foo/bar?baz=1&x=y",
			},
			expectedURL: "http://www.example.com/foo/bar?baz=2&x=y",
		},
		{
			name: "set query when empty",
			context: testContext{
				script: `testdata/set_query.lua`,
				params: []string{"baz", "2"},
				url:    "http://www.example.com/foo/bar",
			},
			expectedURL: "http://www.example.com/foo/bar?baz=2",
		},
		{
			name: "delete query",
			context: testContext{
				script: `testdata/set_query.lua`,
				params: []string{"baz", ""},
				url:    "http://www.example.com/foo/bar?baz=1",
			},
			expectedURL: "http://www.example.com/foo/bar",
		},
		{
			name: "strip query",
			context: testContext{
				script:        `testdata/strip_query.lua`,
				url:           "http://www.example.com/foo/bar?baz=1&x=y&z",
				requestHeader: http.Header{"X-Dummy": []string{"dummy"}},
			},
			expectedURL:           "http://www.example.com/foo/bar",
			expectedRequestHeader: http.Header{"X-Dummy": []string{"dummy"}},
		},
		{
			name: "strip query and preserve to headers",
			context: testContext{
				script: `testdata/strip_query.lua`,
				params: []string{"true"},
				url:    "http://www.example.com/foo/bar?baz=1&x=y&z",
			},
			expectedURL:           "http://www.example.com/foo/bar",
			expectedRequestHeader: http.Header{"X-Query-Param-Baz": []string{"1"}, "X-Query-Param-X": []string{"y"}},
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
				script: `testdata/query_to_header.lua`,
				params: []string{"foo-query-param", "X-Foo-Header"},
				url:    "http://www.example.com/foo/bar?foo-query-param=test",
			},
			expectedRequestHeader: http.Header{"X-Foo-Header": []string{"test"}},
		},
		{
			name: "queryToHeader when header is present",
			context: testContext{
				script:        `testdata/query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar?foo-query-param=test",
				requestHeader: http.Header{"X-Foo-Header": []string{"foo"}},
			},
			expectedRequestHeader: http.Header{"X-Foo-Header": []string{"foo"}},
		},
		{
			name: "queryToHeader when query is absent",
			context: testContext{
				script:        `testdata/query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar",
				requestHeader: http.Header{"X-Dummy": []string{"dummy"}},
			},
			expectedRequestHeader: http.Header{"X-Dummy": []string{"dummy"}},
		},
		{
			name: "queryToHeader when query is empty",
			context: testContext{
				script:        `testdata/query_to_header.lua`,
				params:        []string{"foo-query-param", "X-Foo-Header"},
				url:           "http://www.example.com/foo/bar?foo-query-param=",
				requestHeader: http.Header{"X-Dummy": []string{"dummy"}},
			},
			expectedRequestHeader: http.Header{"X-Dummy": []string{"dummy"}},
		},
		{
			name: "path param to header",
			context: testContext{
				script:     `function request(ctx, params); ctx.request.header["X-Id"] = ctx.path_param["id"]; end`,
				pathParams: map[string]string{"id": "hello"},
			},
			expectedRequestHeader: http.Header{"X-Id": []string{"hello"}},
		},
		{
			name: "get request cookie",
			context: testContext{
				requestHeader: http.Header{"Cookie": []string{"PHPSESSID=298zf09hf012fh2; csrftoken=u32t4o3tb3gg43; _gat=1"}},
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
			opts: LuaOptions{
				// use non-existing module
				Modules: []string{"none"},
			},
			context: testContext{
				requestHeader: http.Header{"Cookie": []string{"PHPSESSID=298zf09hf012fh2; csrftoken=u32t4o3tb3gg43; _gat=1; csrftoken=repeat"}},
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
		{
			name: "disable all modules",
			opts: LuaOptions{
				// use non-existing module
				Modules: []string{"none"},
				Sources: []string{"inline"},
			},
			context: testContext{
				script: `
					function request(ctx, params)
						ctx.request.header["X-Message"] = "still usable without modules"
					end
				`,
			},
			expectedRequestHeader: http.Header{"X-Message": []string{"still usable without modules"}},
		},
		{
			name: "enable inline sources and try to reference inline script",
			opts: LuaOptions{
				Sources: []string{"inline"},
			},
			context: testContext{
				script: `
					function request(ctx, params)
						ctx.request.header["X-Message"] = "test"
					end
				`,
			},
			expectedRequestHeader: http.Header{"X-Message": []string{"test"}},
		},
		{
			name: "enable file sources and try to reference file",
			opts: LuaOptions{
				Sources: []string{"file"},
			},
			context: testContext{
				script: `testdata/query_to_header.lua`,
				params: []string{"foo-query-param", "X-Foo-Header"},
				url:    "http://www.example.com/foo/bar?foo-query-param=test",
			},
			expectedRequestHeader: http.Header{"X-Foo-Header": []string{"test"}},
		},
		{
			name: "enable file and inline sources and try to reference inline script",
			opts: LuaOptions{
				Sources: []string{"file", "inline"},
			},
			context: testContext{
				script: `
					function request(ctx, params)
						ctx.request.header["X-Message"] = "test"
					end
				`,
			},
			expectedRequestHeader: http.Header{"X-Message": []string{"test"}},
		},
		{
			name: "enable file and inline sources and try to reference file",
			opts: LuaOptions{
				Sources: []string{"file", "inline"},
			},
			context: testContext{
				script: `testdata/query_to_header.lua`,
				params: []string{"foo-query-param", "X-Foo-Header"},
				url:    "http://www.example.com/foo/bar?foo-query-param=test",
			},
			expectedRequestHeader: http.Header{"X-Foo-Header": []string{"test"}},
		},
		{
			name: "enable file sources and try to reference inline script",
			opts: LuaOptions{
				Sources: []string{"file"},
			},
			context: testContext{
				script: `
					function request(ctx, params)
						ctx.request.header["X-Message"] = "test"
					end
				`,
			},
			expectedError: fmt.Errorf(`invalid lua source referenced "inline", allowed: "[file]"`),
		},
		{
			name: "enable inline sources and try to reference file",
			opts: LuaOptions{
				Sources: []string{"inline"},
			},
			context: testContext{
				script: `testdata/query_to_header.lua`,
				params: []string{"foo-query-param", "X-Foo-Header"},
				url:    "http://www.example.com/foo/bar?foo-query-param=test",
			},
			expectedError: fmt.Errorf(`invalid lua source referenced "file", allowed: "[inline]"`),
		},
		{
			name: "disable all sources and try to reference inline script",
			opts: LuaOptions{
				Sources: []string{"none"},
			},
			context: testContext{
				script: `
					function request(ctx, params)
						ctx.request.header["X-Message"] = "still usable without modules"
					end
				`,
			},
			expectedError: errLuaSourcesDisabled,
		},
		{
			name: "disable all sources and try to reference file source",
			opts: LuaOptions{
				Sources: []string{"none"},
			},
			context: testContext{
				script: "foo.lua",
			},
			expectedError: errLuaSourcesDisabled,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fc, err := runFilter(test.opts, &test.context)
			if err != nil && test.expectedError == nil {
				t.Fatalf("Failed to run filter: %v", err)
			} else if test.expectedError != nil {
				if err == test.expectedError {
					// ok, error as expected
					return
				} else if err != nil && err.Error() == test.expectedError.Error() {
					// ok, error string as expected
					return
				}
				t.Fatalf("Should fail to create filter: expected: %v, got: %v", test.expectedError, err)
			}

			if test.expectedRequestHeader != nil {
				assert.Equal(t, test.expectedRequestHeader, fc.Request().Header)
			}

			if test.expectedResponseHeader != nil {
				assert.Equal(t, test.expectedResponseHeader, fc.Response().Header)
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
		})
	}
}

func TestSleep(t *testing.T) {
	ctx := &testContext{
		script: `function request(ctx, params) sleep(100.1) end`,
	}
	t0 := time.Now()
	_, err := runFilter(LuaOptions{}, ctx)
	if err != nil {
		t.Fatalf("failed to run filter: %v", err)
	}
	t1 := time.Now()

	if t1.Sub(t0) < 100*time.Millisecond {
		t.Error("expected delay of 100 ms")
	}
}

// testable example have to refer known identifier
const LoadFileOK = `testdata/load_ok.lua`

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

const MalformedFile = `testdata/not_a_filter.lua`

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

const LogHeaders = `testdata/log_header.lua`

func ExampleLogHeaders() {
	runExample(&testContext{
		script: LogHeaders,
		params: []string{"request", "response"},
		// single header as iteration order is not defined
		requestHeader:  http.Header{"X-Foo": []string{"foo"}},
		responseHeader: http.Header{"X-Bar": []string{"bar", "baz"}},
	})
	// Output:
	// GET http://www.example.com/foo/bar HTTP/1.1\r
	// Host: www.example.com\r
	// X-Foo: foo\r
	// \r
	//
	// Response for GET http://www.example.com/foo/bar HTTP/1.1\r
	// 200\r
	// X-Bar: bar baz\r
	// \r
}

const LogRequestAuthHeader = `testdata/log_header.lua`

func ExampleLogRequestAuthHeader() {
	runExample(&testContext{
		script: LogRequestAuthHeader,
		params: []string{"request"},
		// single header as iteration order is not defined
		requestHeader: http.Header{"Authorization": []string{"request secret"}},
	})
	// Output:
	// GET http://www.example.com/foo/bar HTTP/1.1\r
	// Host: www.example.com\r
	// Authorization: TRUNCATED\r
	// \r
}

const MultipleHeaderValues = `
function request(ctx, params)
	ctx.request.header.add("X-Foo", "Bar")
	ctx.request.header.add("X-Foo", "Baz")

	-- all X-Foo values
	for _, v in pairs(ctx.request.header.values("X-Foo")) do
		print(v)
	end

	-- all values
	for k, _ in ctx.request.header() do
		for _, v in pairs(ctx.request.header.values(k)) do
			print(k, "=", v)
		end
	end
end`

func ExampleMultipleHeaderValues() {
	runExample(&testContext{
		script: MultipleHeaderValues,
	})
	// Output:
	// Bar
	// Baz
	// X-Foo=Bar
	// X-Foo=Baz
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

const SetStateBagTable = `
function request(ctx, params)
	ctx.state_bag.table = {foo="bar"}
end

function response(ctx, params)
	print(ctx.state_bag.table.foo)
end
`

func ExampleSetStateBagTable() {
	runExample(&testContext{
		script: SetStateBagTable,
	})
	// Output:
	// bar
}

func newFilter(opts LuaOptions, script string, params ...string) (filters.Filter, error) {
	ls, err := NewLuaScriptWithOptions(opts)
	if err != nil {
		return nil, err
	}
	args := []any{script}
	for _, p := range params {
		args = append(args, p)
	}
	return ls.CreateFilter(args)
}

func runFilter(opts LuaOptions, test *testContext) (filters.FilterContext, error) {
	scr, err := newFilter(opts, test.script, test.params...)
	if err != nil {
		return nil, err
	}
	url := "http://www.example.com/foo/bar"
	if test.url != "" {
		url = test.url
	}
	req, _ := http.NewRequest("GET", url, nil)
	if test.requestHeader != nil {
		req.Header = test.requestHeader.Clone()
	}
	fc := &filtertest.Context{
		FParams:       test.pathParams,
		FStateBag:     make(map[string]any),
		FRequest:      req,
		FOutgoingHost: "www.example.com",
	}
	scr.Request(fc)

	body := "Hello world"
	fc.FResponse = &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Header:        make(http.Header),
	}
	if test.responseHeader != nil {
		fc.Response().Header = test.responseHeader.Clone()
	}

	scr.Response(fc)

	return fc, nil
}

func runExample(ctx *testContext) {
	runExampleWithOptions(LuaOptions{}, ctx)
}

func runExampleWithOptions(opts LuaOptions, ctx *testContext) {
	o := log.StandardLogger().Out
	f := log.StandardLogger().Formatter
	defer func() {
		log.SetOutput(o)
		log.SetFormatter(f)
	}()

	log.SetOutput(os.Stdout)
	log.SetFormatter(&exampleLogFormatter{})

	_, err := runFilter(opts, ctx)
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

	for line := range strings.SplitSeq(entry.Message, "\n") {
		// escape \r to use testable examples
		line = strings.ReplaceAll(line, "\r", `\r`)
		line = strings.TrimRight(line, " ")

		b.WriteString(line)
		b.WriteString("\n")
	}
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

func BenchmarkNewState(b *testing.B) {
	f, _ := newFilter(LuaOptions{}, `function request(ctx, params) end`)
	s := f.(*script)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.newState()
	}
}

func benchmarkRequest(b *testing.B, script string, params ...string) {
	f, _ := newFilter(LuaOptions{}, script, params...)

	r, _ := http.NewRequest("GET", "http://example.com/test", nil)
	r.Header.Add("X-Foo", "Bar")
	fc := &filtertest.Context{FRequest: r}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Request(fc)
	}
}

func BenchmarkScriptRandomPath(b *testing.B) {
	benchmarkRequest(b, "testdata/random_path.lua", "/prefix/", "10")
}

func BenchmarkScriptCopyRequestHeader(b *testing.B) {
	benchmarkRequest(b, "testdata/copy_request_header.lua", "X-Foo", "X-Bar")
}
