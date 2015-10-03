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
