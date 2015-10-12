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
	"fmt"
	"net/http"
	"path"
)

// Filter to serve static content from a file system location. Marks the
// request as served.
//
// It takes the request path prefix from the first argument, clips it from the
// start of the path, appends to the file system root from the second
// argument, and uses the resulting path to serve content from the file
// system.
//
// Implements both Spec and Filter.
type Static struct {
	webRoot, root string
}

// "static"
func (spec *Static) Name() string { return StaticName }

// Creates instances of the Static filter. Expects two argument: request path
// prefix and file system root.
func (spec *Static) CreateFilter(config []interface{}) (Filter, error) {
	if len(config) != 2 {
		return nil, fmt.Errorf("invalid number of args: %d, expected 1", len(config))
	}

	webRoot, ok := config[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid argument type, expected string for web root prefix")
	}

	root, ok := config[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid argument type, expected string for path to root dir")
	}

	return &Static{webRoot, root}, nil
}

// Noop.
func (f *Static) Request(FilterContext) {}

// Serves content from the file system and marks the request served.
func (f *Static) Response(ctx FilterContext) {
	r := ctx.Request()
	p := r.URL.Path

	if len(p) < len(f.webRoot) {
		return
	}

	ctx.MarkServed()
	http.ServeFile(ctx.ResponseWriter(), ctx.Request(), path.Join(f.root, p[len(f.webRoot):]))
}
