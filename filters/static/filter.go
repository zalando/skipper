package static

import (
	"fmt"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"path"
)

const name = "static"

type typ struct {
	id, webRoot, root string
}

func Make() skipper.FilterSpec {
	return &typ{}
}

func (fs *typ) Name() string { return name }

func (fs *typ) MakeFilter(id string, c skipper.FilterConfig) (skipper.Filter, error) {
	if len(c) != 2 {
		return nil, fmt.Errorf("invalid number of args: %d, expected 1", len(c))
	}

	webRoot, ok := c[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid argument type, expected string for web root prefix")
	}

	root, ok := c[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid argument type, expected string for path to root dir")
	}

	return &typ{id, webRoot, root}, nil
}

func (f *typ) Id() string {
	return f.id
}

func (f *typ) Request(skipper.FilterContext) {}

func (f *typ) Response(c skipper.FilterContext) {
	r := c.Request()
	p := r.URL.Path

	if len(p) < len(f.webRoot) {
		return
	}

	c.MarkServed()
	http.ServeFile(c.ResponseWriter(), c.Request(), path.Join(f.root, p[len(f.webRoot):]))
}
