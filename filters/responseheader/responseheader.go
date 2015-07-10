// filter to set a preconfigured header for a response.
// the name of the header is expected in the 'key' field of the filter config, and the value of the header in the
// 'value' field.
package responseheader

import (
	"github.com/zalando/skipper/filters/simpleheader"
	"github.com/zalando/skipper/skipper"
)

const name = "responseHeader"

type impl struct {
	simpleheader.Type
}

// creates the filter spec instance
func Make() skipper.FilterSpec {
	return &impl{}
}

// returns the name of the filter spec
func (mw *impl) Name() string {
	return name
}

// creates a filter instance with the provided header key and value in the filter config.
func (mw *impl) MakeFilter(id string, config skipper.FilterConfig) (skipper.Filter, error) {
	f := &impl{}
	err := f.InitFilter(id, config)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// sets the configured header for the response
func (f *impl) Response(ctx skipper.FilterContext) {
	ctx.Response().Header.Add(f.Key(), f.Value())
}
