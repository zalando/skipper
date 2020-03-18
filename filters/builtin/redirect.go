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
	"net/http"
	"net/url"
	"strings"

	"github.com/zalando/skipper/filters"
)

type redirectType int

const (
	redTo redirectType = iota
	redToLower
)

// Filter to return
type redirect struct {
	typ      redirectType
	code     int
	location *url.URL
}

// NewRedirectTo returns a new filter Spec, whose instances create an HTTP redirect
// response. It shunts the request flow, meaning that the filter chain on
// the request path is not continued. The request is not forwarded to the
// backend. Instances expect two parameters: the redirect status code and
// the redirect location.
// Name: "redirectTo".
func NewRedirectTo() filters.Spec { return &redirect{typ: redTo} }

// NewRedirectLower returns a new filter Spec, whose instances create an HTTP redirect
// response, which redirects with a lowercase path. It is similar to redTo except that
// it converts the route path to lower while redirecting
// Name: "redirectToLower".
func NewRedirectLower() filters.Spec { return &redirect{typ: redToLower} }

// "redirect" or "redirectToLower" or "redirectTo"
func (spec *redirect) Name() string {
	switch spec.typ {
	case redToLower:
		return RedirectToLowerName
	default:
		return RedirectToName
	}
}

// Creates an instance of the redirect filter.
func (spec *redirect) CreateFilter(config []interface{}) (filters.Filter, error) {
	invalidArgs := func() (filters.Filter, error) {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(config) == 1 {
		config = append(config, "")
	}

	if len(config) != 2 {
		return invalidArgs()
	}

	code, ok := config[0].(float64)
	if !ok {
		return invalidArgs()
	}

	location, ok := config[1].(string)
	if !ok {
		return invalidArgs()
	}

	u, err := url.Parse(location)
	if err != nil {
		return invalidArgs()
	}

	return &redirect{spec.typ, int(code), u}, nil
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

func getLocation(ctx filters.FilterContext, location *url.URL, typ redirectType) string {
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

	// Check if the redirect has to be case-insensitive
	if typ == redToLower {
		u.Path = strings.ToLower(u.Path)
	}

	if u.RawQuery == "" {
		u.RawQuery = r.URL.RawQuery
	}

	return u.String()
}

func redirectWithType(ctx filters.FilterContext, code int, location *url.URL, typ redirectType) {
	u := getLocation(ctx, location, typ)
	ctx.Serve(&http.Response{
		StatusCode: code,
		Header:     http.Header{"Location": []string{u}}})
}

// Redirect implements the redirect logic as a standalone function.
func Redirect(ctx filters.FilterContext, code int, location *url.URL) {
	redirectWithType(ctx, code, location, redTo)
}

func (spec *redirect) Request(ctx filters.FilterContext) {
	redirectWithType(ctx, spec.code, spec.location, spec.typ)
}

// Sets the status code and the location header of the response. Marks the
// request served.
func (spec *redirect) Response(ctx filters.FilterContext) {
	u := getLocation(ctx, spec.location, spec.typ)
	w := ctx.ResponseWriter()
	w.Header().Set("Location", u)
	w.WriteHeader(spec.code)
	ctx.MarkServed()
}
