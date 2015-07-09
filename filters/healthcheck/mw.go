package healthcheck

import (
	"github.com/zalando/skipper/filters/noop"
	"github.com/zalando/skipper/skipper"
)

const name = "healthcheck"

type typ struct {
	*noop.Type
}

func Make() skipper.FilterSpec {
	return &typ{}
}

func (h *typ) Name() string {
	return name
}

func (h *typ) MakeFilter(id string, _ skipper.FilterConfig) (skipper.Filter, error) {
	hf := &typ{&noop.Type{}}
	hf.SetId(id)
	return hf, nil
}

func (h *typ) Response(ctx skipper.FilterContext) {
	ctx.Response().StatusCode = 200
}
