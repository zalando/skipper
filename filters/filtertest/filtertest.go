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

/*
Package filtertest implements mock versions of the Filter, Spec and
FilterContext interfaces used during tests.
*/
package filtertest

import (
	"github.com/zalando/skipper/filters"
	"net/http"
)

// Noop filter, used to verify the filter name and the args in the route.
// Implements both the Filter and the Spec interfaces.
type Filter struct {
	FilterName string
	Args       []interface{}
}

// Simple FilterContext implementation.
type Context struct {
	FResponseWriter     http.ResponseWriter
	FRequest            *http.Request
	FResponse           *http.Response
	FServed             bool
	FServedWithResponse bool
	FParams             map[string]string
	FStateBag           map[string]interface{}
	FBackendUrl         string
	FOutgoingHost       string
	FMetrics            *filters.FilterMetrics
}

func (spec *Filter) Name() string                    { return spec.FilterName }
func (f *Filter) Request(ctx filters.FilterContext)  {}
func (f *Filter) Response(ctx filters.FilterContext) {}

func (fc *Context) ResponseWriter() http.ResponseWriter { return fc.FResponseWriter }
func (fc *Context) Request() *http.Request              { return fc.FRequest }
func (fc *Context) Response() *http.Response            { return fc.FResponse }
func (fc *Context) MarkServed()                         { fc.FServed = true }
func (fc *Context) Served() bool                        { return fc.FServed }
func (fc *Context) PathParam(key string) string         { return fc.FParams[key] }
func (fc *Context) StateBag() map[string]interface{}    { return fc.FStateBag }
func (fc *Context) OriginalRequest() *http.Request      { return nil }
func (fc *Context) OriginalResponse() *http.Response    { return nil }
func (fc *Context) BackendUrl() string                  { return fc.FBackendUrl }
func (fc *Context) OutgoingHost() string                { return fc.FOutgoingHost }
func (fc *Context) SetOutgoingHost(h string)            { fc.FOutgoingHost = h }
func (fc *Context) Metrics() *filters.FilterMetrics     { return fc.FMetrics }
func (fc *Context) Serve(resp *http.Response) {
	fc.FServedWithResponse = true
	fc.FResponse = resp
	fc.FServed = true
}

func (spec *Filter) CreateFilter(config []interface{}) (filters.Filter, error) {
	return &Filter{spec.FilterName, config}, nil
}
