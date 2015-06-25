// filter to set a preconfigured header for a request.
// the name of the header is expected in the 'key' field of the filter config, and the value of the header in the
// 'value' field.
// if the header key is called 'Host', it sets the request object's Host field, too.
package requestheader

import (
	"skipper/filters/simpleheader"
	"skipper/skipper"
)

const name = "request-header"

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

// sets the configured header for the request
func (f *impl) Request(ctx skipper.FilterContext) {
	req := ctx.Request()
	if f.Key() == "Host" {
		req.Host = f.Value()
	}

	req.Header.Add(f.Key(), f.Value())
}
