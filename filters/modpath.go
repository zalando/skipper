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

// provides a filter that can rewrite the request path.
//
// the filters expect a regular expression in the 'expression' field of the filter config to match one or more parts of the request
// path, and a replacement string in the 'replacement' field. when processing a request, it calls ReplaceAll on
// the path.
package filters

import (
	"fmt"
	"regexp"
)

const ModPathName = "modPath"

type ModPath struct {
	rx          *regexp.Regexp
	replacement []byte
}

func (spec *ModPath) Name() string { return ModPathName }

func invalidConfig(config []interface{}) error {
	return fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", ModPathName, config)
}

func (spec *ModPath) CreateFilter(config []interface{}) (Filter, error) {
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

	f := &ModPath{rx, []byte(replacement)}
	return f, nil
}

func (f *ModPath) Request(ctx FilterContext) {
	req := ctx.Request()
	req.URL.Path = string(f.rx.ReplaceAll([]byte(req.URL.Path), f.replacement))
}

func (f *ModPath) Response(ctx FilterContext) {}
