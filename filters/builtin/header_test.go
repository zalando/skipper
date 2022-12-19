package builtin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
)

type testContext struct {
	key, value string
}

func (c testContext) Name() string { return "testContext" }

func (c testContext) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	key, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	value, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return testContext{key, value}, nil
}

func (c testContext) Request(ctx filters.FilterContext) {
	ctx.StateBag()[c.key] = c.value
}

func (c testContext) Response(filters.FilterContext) {}

func printHeader(t *testing.T, h http.Header, msg ...interface{}) {
	for k, v := range h {
		for _, vi := range v {
			t.Log(append(msg, k, vi)...)
		}
	}
}

func compareHeaders(left, right http.Header) bool {
	if len(left) != len(right) {
		return false
	}

	for k, v := range left {
		vright := right[k]
		if len(v) != len(vright) {
			return false
		}

		for _, vi := range v {
			found := false
			for _, vri := range vright {
				if vri == vi {
					found = true
					break
				}
			}

			if !found {
				return false
			}
		}
	}

	return true
}

func testHeaders(t *testing.T, got, expected http.Header) {
	for n := range got {
		if !strings.HasPrefix(n, "X-Test-") {
			delete(got, n)
		}
	}

	if !compareHeaders(got, expected) {
		printHeader(t, expected, "invalid header", "expected")
		printHeader(t, got, "invalid header", "got")
		t.Error("invalid header")
	}
}

func TestHeader(t *testing.T) {
	type testItem struct {
		msg            string
		args           []interface{}
		context        map[string]interface{}
		host           string
		pathPredicate  string
		path           string
		valid          bool
		requestHeader  http.Header
		responseHeader http.Header
		expectedHeader http.Header
	}

	for filter, tests := range map[string][]testItem{
		"setRequestHeader": {{
			msg:   "invalid number of args",
			args:  []interface{}{"name", "value", "other value"},
			valid: false,
		}, {
			msg:   "name not string",
			args:  []interface{}{3, "value"},
			valid: false,
		}, {
			msg:   "value not string",
			args:  []interface{}{"name", 3},
			valid: false,
		}, {
			msg:            "set request header when none",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:            "set request header when exists",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:   "set outgoing host on set",
			args:  []interface{}{"Host", "www.example.org"},
			valid: true,
			host:  "www.example.org",
		}, {
			msg:            "set request header from path params",
			args:           []interface{}{"X-Test-Name", "Mit ${was} zu ${wo}"},
			pathPredicate:  "/path/:was/:wo",
			path:           "/path/Raketen/Planeten",
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"Mit Raketen zu Planeten"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-name", "Value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"Value"}},
		}},
		"appendRequestHeader": {{
			msg:            "append request header when none",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:            "append request header when exists",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value0", "value1", "value"}},
		}, {
			msg:   "append outgoing host on set",
			args:  []interface{}{"Host", "www.example.org"},
			valid: true,
			host:  "www.example.org",
		}, {
			msg:            "append request header from path params",
			args:           []interface{}{"X-Test-Name", "a ${foo}ter"},
			pathPredicate:  "/path/:foo",
			path:           "/path/bar",
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value0", "value1", "a barter"}},
		}, {
			msg:            "append request header from path params when missing",
			args:           []interface{}{"X-Test-Name", "${foo}"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value0", "value1"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-name", "Value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"Value"}},
		}},
		"dropRequestHeader": {{
			msg:   "drop request header when none",
			args:  []interface{}{"X-Test-Name"},
			valid: true,
		}, {
			msg:           "drop request header when exists",
			args:          []interface{}{"X-Test-Name"},
			valid:         true,
			requestHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}, {
			msg:           "name parameter is case-insensitive",
			args:          []interface{}{"x-test-name"},
			valid:         true,
			requestHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}},
		"setResponseHeader": {{
			msg:            "set response header when none",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}, {
			msg:            "set response header when exists",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}, {
			msg:            "set response header from path params",
			args:           []interface{}{"X-Test-Name", "a ${sizeof} ${foo}ter"},
			pathPredicate:  "/path/:sizeof/:foo",
			path:           "/path/small/bar",
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"a small barter"}},
		}, {
			msg:   "set response header from path params when missing",
			args:  []interface{}{"X-Test-Name", "a ${foo}ter"},
			valid: true,
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-name", "Value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"Value"}},
		}},
		"appendResponseHeader": {{
			msg:            "append response header when none",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}, {
			msg:            "append response header when exists",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value0", "value1", "value"}},
		}, {
			msg:            "append response header from path params",
			args:           []interface{}{"X-Test-Name", "a ${foo}ter"},
			pathPredicate:  "/path/:foo",
			path:           "/path/bar",
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value0", "value1", "a barter"}},
		}, {
			msg:            "append response header from path params when missing",
			args:           []interface{}{"X-Test-Name", "a ${foo}ter"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-name", "Value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"Value"}},
		}},
		"dropResponseHeader": {{
			msg:   "drop response header when none",
			args:  []interface{}{"X-Test-Name"},
			valid: true,
		}, {
			msg:            "drop response header when exists",
			args:           []interface{}{"X-Test-Name"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-name"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}},
		"setContextRequestHeader": {{
			msg:            "set request header from context",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar"}},
		}, {
			msg:     "set request host header from context",
			args:    []interface{}{"Host", "foo"},
			context: map[string]interface{}{"foo": "www.example.org"},
			valid:   true,
			host:    "www.example.org",
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar"}},
		}},
		"appendContextRequestHeader": {{
			msg:            "append request header from context",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar", "baz"}},
		}, {
			msg:     "append request host header from context",
			args:    []interface{}{"Host", "foo"},
			context: map[string]interface{}{"foo": "www.example.org"},
			valid:   true,
			host:    "www.example.org",
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar", "baz"}},
		}},
		"setContextResponseHeader": {{
			msg:            "set response header from context",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar"}},
		}},
		"appendContextResponseHeader": {{
			msg:            "append response header from context",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar", "baz"}},
		}, {
			msg:            "name parameter is case-insensitive",
			args:           []interface{}{"x-test-foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar", "baz"}},
		}},
		"copyRequestHeader": {{
			msg:  "too few args",
			args: []interface{}{"X-Test-Foo"},
		}, {
			msg:  "too many args",
			args: []interface{}{"X-Test-Foo", "X-Test-Bar", "baz"},
		}, {
			msg:  "invalid source header name",
			args: []interface{}{42, "X-Test-Bar"},
		}, {
			msg:  "invalid target header name",
			args: []interface{}{"X-Test-Foo", 42},
		}, {
			msg:   "no header to copy",
			args:  []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid: true,
		}, {
			msg:           "copy header",
			args:          []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid:         true,
			requestHeader: http.Header{"X-Test-Foo": []string{"foo"}},
			expectedHeader: http.Header{
				"X-Test-Request-Foo": []string{"foo"},
				"X-Test-Request-Bar": []string{"foo"},
			},
		}, {
			msg:   "overwrite header",
			args:  []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid: true,
			requestHeader: http.Header{
				"X-Test-Foo": []string{"foo"},
				"X-Test-Bar": []string{"bar"},
			},
			expectedHeader: http.Header{
				"X-Test-Request-Foo": []string{"foo"},
				"X-Test-Request-Bar": []string{"foo"},
			},
		}, {
			msg:   "host header",
			args:  []interface{}{"X-Test-Source-Host", "Host"},
			valid: true,
			host:  "www.example.org",
			requestHeader: http.Header{
				"X-Test-Source-Host": []string{"www.example.org"},
			},
			expectedHeader: http.Header{
				"X-Test-Request-Source-Host": []string{"www.example.org"},
			},
		}, {
			msg:           "name parameters are case-insensitive",
			args:          []interface{}{"x-test-foo", "x-test-bar"},
			valid:         true,
			requestHeader: http.Header{"X-Test-Foo": []string{"foo"}},
			expectedHeader: http.Header{
				"X-Test-Request-Foo": []string{"foo"},
				"X-Test-Request-Bar": []string{"foo"},
			},
		}},
		"copyResponseHeader": {{
			msg:  "too few args",
			args: []interface{}{"X-Test-Foo"},
		}, {
			msg:  "too many args",
			args: []interface{}{"X-Test-Foo", "X-Test-Bar", "baz"},
		}, {
			msg:  "invalid source header name",
			args: []interface{}{42, "X-Test-Bar"},
		}, {
			msg:  "invalid target header name",
			args: []interface{}{"X-Test-Foo", 42},
		}, {
			msg:   "no header to copy",
			args:  []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid: true,
		}, {
			msg:            "copy header",
			args:           []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Foo": []string{"foo"}},
			expectedHeader: http.Header{
				"X-Test-Foo": []string{"foo"},
				"X-Test-Bar": []string{"foo"},
			},
		}, {
			msg:   "overwrite header",
			args:  []interface{}{"X-Test-Foo", "X-Test-Bar"},
			valid: true,
			responseHeader: http.Header{
				"X-Test-Foo": []string{"foo"},
				"X-Test-Bar": []string{"bar"},
			},
			expectedHeader: http.Header{
				"X-Test-Foo": []string{"foo"},
				"X-Test-Bar": []string{"foo"},
			},
		}, {
			msg:            "name parameters are case-insensitive",
			args:           []interface{}{"x-test-foo", "x-test-bar"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Foo": []string{"foo"}},
			expectedHeader: http.Header{
				"X-Test-Foo": []string{"foo"},
				"X-Test-Bar": []string{"foo"},
			},
		}}} {
		t.Run(filter, func(t *testing.T) {
			for _, ti := range tests {
				t.Run(ti.msg, func(t *testing.T) {
					bs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						for n, vs := range r.Header {
							if strings.HasPrefix(n, "X-Test-") {
								w.Header()["X-Test-Request-"+n[7:]] = vs
							}
						}

						for n, vs := range ti.responseHeader {
							w.Header()[n] = vs
						}

						w.Header().Set("X-Request-Host", r.Host)
					}))
					defer bs.Close()

					fr := make(filters.Registry)
					fr.Register(NewSetRequestHeader())
					fr.Register(NewAppendRequestHeader())
					fr.Register(NewDropRequestHeader())
					fr.Register(NewSetResponseHeader())
					fr.Register(NewAppendResponseHeader())
					fr.Register(NewDropResponseHeader())
					fr.Register(NewSetContextRequestHeader())
					fr.Register(NewAppendContextRequestHeader())
					fr.Register(NewSetContextResponseHeader())
					fr.Register(NewAppendContextResponseHeader())
					fr.Register(NewCopyRequestHeader())
					fr.Register(NewCopyResponseHeader())
					fr.Register(testContext{})

					filters := []*eskip.Filter{{Name: filter, Args: ti.args}}
					for key, value := range ti.context {
						filters = append([]*eskip.Filter{{
							Name: "testContext",
							Args: []interface{}{key, value},
						}}, filters...)
					}

					r := &eskip.Route{
						Filters: filters,
						Backend: bs.URL,
					}

					if ti.pathPredicate != "" {
						r.Predicates = append(r.Predicates, &eskip.Predicate{Name: "Path", Args: []interface{}{ti.pathPredicate}})
					}

					pr := proxytest.New(fr, r)
					defer pr.Close()

					path := pr.URL
					if ti.path != "" {
						path += ti.path
					}

					req, err := http.NewRequest("GET", path, nil)
					if err != nil {
						t.Error(err)
						return
					}

					req.Close = true

					for n, vs := range ti.requestHeader {
						req.Header[n] = vs
					}

					rsp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					defer rsp.Body.Close()

					if ti.valid && rsp.StatusCode != http.StatusOK ||
						!ti.valid && rsp.StatusCode != http.StatusNotFound {
						t.Errorf("failed to validate arguments, valid: %v, status: %v", ti.valid, rsp.StatusCode)
						return
					}

					if ti.host != "" && ti.host != rsp.Header.Get("X-Request-Host") {
						t.Error("failed to set outgoing request host")
					}

					if ti.valid {
						testHeaders(t, rsp.Header, ti.expectedHeader)
					}
				})
			}
		})
	}
}

func BenchmarkCopyRequestHeader(b *testing.B) {
	spec := NewCopyRequestHeader()
	f, _ := spec.CreateFilter([]interface{}{"X-Foo", "X-Bar"})

	r, _ := http.NewRequest("GET", "http://example.com", nil)
	r.Header.Add("X-Foo", "whatever")
	fc := &filtertest.Context{FRequest: r}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Request(fc)
	}
}
