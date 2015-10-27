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
	"fmt"
	"github.com/zalando/skipper/filters"
	"net/http"
	"path"
)

type static struct {
	webRoot, root string
}

// Returns a filter Spec to serve static content from a file system
// location. Marks the request as served.
//
// Filter instances of this specification expect two parameters: a
// request path prefix and a local directory path. When processing a
// request, it clips the prefix from the request path, and appends the
// rest of the path to the directory path. Then, it uses the resulting
// path to serve static content from the file system.
//
// Name: "static".
func NewStatic() filters.Spec { return &static{} }

// "static"
func (spec *static) Name() string { return StaticName }

// Creates instances of the static filter. Expects two parameters: request path
// prefix and file system root.
func (spec *static) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 2 {
		return nil, fmt.Errorf("invalid number of args: %d, expected 1", len(config))
	}

	webRoot, ok := config[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid parameter type, expected string for web root prefix")
	}

	root, ok := config[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid parameter type, expected string for path to root dir")
	}

	return &static{webRoot, root}, nil
}

// Noop.
func (f *static) Request(filters.FilterContext) {}

// Serves content from the file system and marks the request served.
func (f *static) Response(ctx filters.FilterContext) {
	r := ctx.Request()
	p := r.URL.Path

	if len(p) < len(f.webRoot) {
		return
	}

	ctx.MarkServed()
	http.ServeFile(ctx.ResponseWriter(), ctx.Request(), path.Join(f.root, p[len(f.webRoot):]))
}
