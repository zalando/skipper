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

type ModPath struct {
	rx          *regexp.Regexp
	replacement []byte
}

func (spec *ModPath) Name() string { return "modPath" }

func invalidConfig(config []interface{}) error {
	return fmt.Errorf("invalid filter config in %s, expecting regexp and string, got: %v", "modPath", config)
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
