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

package filters

import (
	"errors"
	"net/url"
)

// Filter to return an HTTP redirect resposne. Marks the request as served.
// Implements both Spec and Filter.
type Redirect struct {
	code     int
	location *url.URL
}

// "redirect"
func (spec *Redirect) Name() string { return RedirectName }

// Creates an instance of the Redirect filter. Expects two arguments: the
// redirect status code and the redirect location.
func (spec *Redirect) CreateFilter(config []interface{}) (Filter, error) {
	invalidArgs := func() (Filter, error) {
		return nil, errors.New("invalid arguments")
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

	return &Redirect{int(code), u}, nil
}

// Noop.
func (f *Redirect) Request(ctx FilterContext) {}

// Sets the status code and the location header of the response. Marks the
// request served.
func (f *Redirect) Response(ctx FilterContext) {
	w := ctx.ResponseWriter()

	u := *f.location
	if u.Host == "" {
		u.Scheme = ctx.Request().URL.Scheme
		u.Host = ctx.Request().URL.Host
	}

	w.Header().Set("Location", (&u).String())
	w.WriteHeader(f.code)
	ctx.MarkServed()
}
