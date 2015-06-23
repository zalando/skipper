package pathrewrite

import (
	"regexp"
	"skipper/middleware/noop"
	"skipper/skipper"
)

const name = "path-rewrite"

type impl struct {
	*noop.Type
	rx          *regexp.Regexp
	replacement []byte
}

func Make() skipper.Middleware {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}

func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
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

func (f *impl) Request(ctx skipper.FilterContext) {
	req := ctx.Request()
	req.URL.Path = string(f.rx.ReplaceAll([]byte(req.URL.Path), f.replacement))
}
