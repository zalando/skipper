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
	code     int
	location *url.URL
}

// Returns a new filter Spec, whose instances create an HTTP redirect
// resposne. Marks the request as served. Instances expect two
// parameters: the redirect status code and the redirect location.
func NewRedirect() filters.Spec { return &redirect{} }

// "redirect"
func (spec *redirect) Name() string { return RedirectName }

// Creates an instance of the redirect filter.
func (spec *redirect) CreateFilter(config []interface{}) (filters.Filter, error) {
	invalidArgs := func() (filters.Filter, error) {
		return nil, filters.ErrInvalidFilterParameters
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

	return &redirect{int(code), u}, nil
}

// Noop.
func (f *redirect) Request(ctx filters.FilterContext) {}

func (f *redirect) copyOfLocation() *url.URL {
	v := *f.location
	return &v
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

// Sets the status code and the location header of the response. Marks the
// request served.
func (f *redirect) Response(ctx filters.FilterContext) {
	r := ctx.Request()
	w := ctx.ResponseWriter()
	u := f.copyOfLocation()

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

	w.Header().Set("Location", u.String())
	w.WriteHeader(f.code)
	ctx.MarkServed()
}
