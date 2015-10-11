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

type Static struct {
	webRoot, root string
}

func (spec *Static) Name() string { return "static" }

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

func (f *Static) Request(FilterContext) {}

func (f *Static) Response(ctx FilterContext) {
	r := ctx.Request()
	p := r.URL.Path

	if len(p) < len(f.webRoot) {
		return
	}

	ctx.MarkServed()
	http.ServeFile(ctx.ResponseWriter(), ctx.Request(), path.Join(f.root, p[len(f.webRoot):]))
}
