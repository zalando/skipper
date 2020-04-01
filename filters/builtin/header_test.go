// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtin

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		filterName     string
		args           []interface{}
		context        map[string]interface{}
		host           string
		valid          bool
		requestHeader  http.Header
		responseHeader http.Header
		expectedHeader http.Header
	}

	for filter, tests := range map[string][]testItem{
		"setRequestHeader": {{
			msg:        "invalid number of args",
			filterName: "setRequestHeader",
			args:       []interface{}{"name", "value", "other value"},
			valid:      false,
		}, {
			msg:        "name not string",
			filterName: "setRequestHeader",
			args:       []interface{}{3, "value"},
			valid:      false,
		}, {
			msg:        "value not string",
			filterName: "setRequestHeader",
			args:       []interface{}{"name", 3},
			valid:      false,
		}, {
			msg:            "set request header when none",
			filterName:     "setRequestHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:            "set request header when exists",
			filterName:     "setRequestHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:        "set outgoing host on set",
			filterName: "setRequestHeader",
			args:       []interface{}{"Host", "www.example.org"},
			valid:      true,
			host:       "www.example.org",
		}},
		"appendRequestHeader": {{
			msg:            "append request header when none",
			filterName:     "appendRequestHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value"}},
		}, {
			msg:            "append request header when exists",
			filterName:     "appendRequestHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Request-Name": []string{"value0", "value1", "value"}},
		}, {
			msg:        "append outgoing host on set",
			filterName: "appendRequestHeader",
			args:       []interface{}{"Host", "www.example.org"},
			valid:      true,
			host:       "www.example.org",
		}},
		"dropRequestHeader": {{
			msg:        "drop request header when none",
			filterName: "dropRequestHeader",
			args:       []interface{}{"X-Test-Name"},
			valid:      true,
		}, {
			msg:           "drop request header when exists",
			filterName:    "dropRequestHeader",
			args:          []interface{}{"X-Test-Name"},
			valid:         true,
			requestHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}},
		"setResponseHeader": {{
			msg:            "set response header when none",
			filterName:     "setResponseHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}, {
			msg:            "set response header when exists",
			filterName:     "setResponseHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}},
		"appendResponseHeader": {{
			msg:            "append response header when none",
			filterName:     "appendResponseHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Name": []string{"value"}},
		}, {
			msg:            "append response header when exists",
			filterName:     "appendResponseHeader",
			args:           []interface{}{"X-Test-Name", "value"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
			expectedHeader: http.Header{"X-Test-Name": []string{"value0", "value1", "value"}},
		}},
		"dropResponseHeader": {{
			msg:        "drop response header when none",
			filterName: "dropResponseHeader",
			args:       []interface{}{"X-Test-Name"},
			valid:      true,
		}, {
			msg:            "drop response header when exists",
			filterName:     "dropResponseHeader",
			args:           []interface{}{"X-Test-Name"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Name": []string{"value0", "value1"}},
		}},
		"setContextRequestHeader": {{
			msg:            "set request header from context",
			filterName:     "setContextRequestHeader",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar"}},
		}, {
			msg:        "set request host header from context",
			filterName: "setContextRequestHeader",
			args:       []interface{}{"Host", "foo"},
			context:    map[string]interface{}{"foo": "www.example.org"},
			valid:      true,
			host:       "www.example.org",
		}},
		"appendContextRequestHeader": {{
			msg:            "append request header from context",
			filterName:     "appendContextRequestHeader",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			requestHeader:  http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Request-Foo": []string{"bar", "baz"}},
		}, {
			msg:        "append request host header from context",
			filterName: "appendContextRequestHeader",
			args:       []interface{}{"Host", "foo"},
			context:    map[string]interface{}{"foo": "www.example.org"},
			valid:      true,
			host:       "www.example.org",
		}},
		"setContextResponseHeader": {{
			msg:            "set response header from context",
			filterName:     "setContextResponseHeader",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "bar"},
			valid:          true,
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar"}},
		}},
		"appendContextResponseHeader": {{
			msg:            "append response header from context",
			filterName:     "appendContextResponseHeader",
			args:           []interface{}{"X-Test-Foo", "foo"},
			context:        map[string]interface{}{"foo": "baz"},
			valid:          true,
			responseHeader: http.Header{"X-Test-Foo": []string{"bar"}},
			expectedHeader: http.Header{"X-Test-Foo": []string{"bar", "baz"}},
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
					fr.Register(testContext{})

					filters := []*eskip.Filter{{Name: ti.filterName, Args: ti.args}}
					for key, value := range ti.context {
						filters = append([]*eskip.Filter{{
							Name: "testContext",
							Args: []interface{}{key, value},
						}}, filters...)
					}

					pr := proxytest.New(fr, &eskip.Route{
						Filters: filters,
						Backend: bs.URL},
					)
					defer pr.Close()

					req, err := http.NewRequest("GET", pr.URL, nil)
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

					if ti.valid && rsp.StatusCode != http.StatusOK ||
						!ti.valid && rsp.StatusCode != http.StatusNotFound {
						t.Error("failed to validate arguments")
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
