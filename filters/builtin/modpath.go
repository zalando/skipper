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
	"regexp"
)

type modPath struct {
	rx          *regexp.Regexp
	replacement []byte
}

// Returns a new modpath filter Spec, whose instances execute
// regexp.ReplaceAll on the request path. Instances expect two
// parameters: the expression to match and the replacement string.
// Name: "modpath".
func NewModPath() filters.Spec { return &modPath{} }

// "modPath"
func (spec *modPath) Name() string { return ModPathName }

func invalidConfig(config []interface{}) error {
	return fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", ModPathName, config)
}

// Creates instances of the modPath filter.
func (spec *modPath) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 2 {
		return nil, invalidConfig(config)
	}

	expr, ok := config[0].(string)
	if !ok {
		return nil, invalidConfig(config)
	}

	replacement, ok := config[1].(string)
	if !ok {
		return nil, invalidConfig(config)
	}

	rx, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	f := &modPath{rx, []byte(replacement)}
	return f, nil
}

// Modifies the path with regexp.ReplaceAll.
func (f *modPath) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	req.URL.Path = string(f.rx.ReplaceAll([]byte(req.URL.Path), f.replacement))
}

// Noop.
func (f *modPath) Response(ctx filters.FilterContext) {}
