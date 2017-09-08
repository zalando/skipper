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
	"github.com/zalando/skipper/filters"
	"net/http"
	"net/url"
)

// Filter to return
type redirect struct {
	deprecated bool
	code       int
	location   *url.URL
}

// Returns a new filter Spec, whose instances create an HTTP redirect
// response. Marks the request as served. Instances expect two
// parameters: the redirect status code and the redirect location.
// Name: "redirect".
//
// This filter is deprecated, use RedirectTo instead.
func NewRedirect() filters.Spec { return &redirect{deprecated: true} }

// Returns a new filter Spec, whose instances create an HTTP redirect
// response. It shunts the request flow, meaning that the filter chain on
// the request path is not continued. The request is not forwarded to the
// backend. Instances expect two parameters: the redirect status code and
// the redirect location.
// Name: "redirectTo".
func NewRedirectTo() filters.Spec { return &redirect{deprecated: false} }

// "redirect" or "redirectTo"
func (spec *redirect) Name() string {
	if spec.deprecated {
		return RedirectName
	} else {
		return RedirectToName
	}
}

// Creates an instance of the redirect filter.
func (spec *redirect) CreateFilter(config []interface{}) (filters.Filter, error) {
	invalidArgs := func() (filters.Filter, error) {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(config) != 2 {
		return invalidArgs()
	}

	var statusCode int64
	var httpCode int
	code, ok := config[0].(float64)
	if !ok {
		statusCode, ok = config[0].(int64)
		if !ok {
			return invalidArgs()
		}
	}

	if code == 0 {
		httpCode = int(statusCode)
	} else {
		httpCode = int(code)
	}

	location, ok := config[1].(string)
	if !ok {
		return invalidArgs()
	}

	u, err := url.Parse(location)
	if err != nil {
		return invalidArgs()
	}

	return &redirect{spec.deprecated, httpCode, u}, nil
}

func getRequestHost(r *http.Request) string {
	h := r.Header.Get("Host")

	if h == "" {
		h = r.Host
	}

	if h == "" {
		h = r.URL.Host
	}

	return h
}

func getLocation(ctx filters.FilterContext, location *url.URL) string {
	r := ctx.Request()

	uc := *location
	u := &uc

	if u.Scheme == "" {
		if r.URL.Scheme != "" {
			u.Scheme = r.URL.Scheme
		} else {
			u.Scheme = "https"
		}
	}

	u.User = r.URL.User

	if u.Host == "" {
		u.Host = getRequestHost(r)
	}

	if u.Path == "" {
		u.Path = r.URL.Path
	}

	if u.RawQuery == "" {
		u.RawQuery = r.URL.RawQuery
	}

	return u.String()
}

// Redirect implements the redirect logic as a standalone function.
func Redirect(ctx filters.FilterContext, code int, location *url.URL) {
	u := getLocation(ctx, location)
	ctx.Serve(&http.Response{
		StatusCode: code,
		Header:     http.Header{"Location": []string{u}}})
}

func (f *redirect) Request(ctx filters.FilterContext) {
	if f.deprecated {
		return
	}

	Redirect(ctx, f.code, f.location)
}

// Sets the status code and the location header of the response. Marks the
// request served.
func (f *redirect) Response(ctx filters.FilterContext) {
	if !f.deprecated {
		return
	}

	u := getLocation(ctx, f.location)
	w := ctx.ResponseWriter()
	w.Header().Set("Location", u)
	w.WriteHeader(f.code)
	ctx.MarkServed()
}
