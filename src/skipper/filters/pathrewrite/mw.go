// creates a filter that can create filters that can rewrite the request path.
// the filters expect a regular expression in the 'expression' field of the filter config to match one or more parts of the request
// path, and a replacement string in the 'replacement' field. when processing a request, it calls ReplaceAll on
// the path.
package pathrewrite

import (
	"regexp"
	"skipper/filters/noop"
	"skipper/skipper"
)

const name = "path-rewrite"

type impl struct {
	*noop.Type
	rx          *regexp.Regexp
	replacement []byte
}

// creates the filter spec instance
func Make() skipper.FilterSpec {
	return &impl{}
}

// the name of the filter spec
func (mw *impl) Name() string {
	return name
}

// creates a path rewrite filter
func (mw *impl) MakeFilter(id string, config skipper.FilterConfig) (skipper.Filter, error) {
	expr, _ := config["expression"].(string)
	replacement, _ := config["replacement"].(string)

	rx, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	f := &impl{&noop.Type{}, rx, []byte(replacement)}
	f.SetId(id)
	return f, nil
}

// rewrites the path of the filter
func (f *impl) Request(ctx skipper.FilterContext) {
	req := ctx.Request()
	req.URL.Path = string(f.rx.ReplaceAll([]byte(req.URL.Path), f.replacement))
}
