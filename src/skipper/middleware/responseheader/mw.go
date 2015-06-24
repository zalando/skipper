// middleware to set a preconfigured header for a response.
// the name of the header is expected in the 'key' field of the filter config, and the value of the header in the
// 'value' field.
package responseheader

import (
	"skipper/middleware/simpleheader"
	"skipper/skipper"
)

const name = "response-header"

type impl struct {
	simpleheader.Type
}

// creates the middleware instance
func Make() skipper.Middleware {
	return &impl{}
}

// returns the name of the middleware
func (mw *impl) Name() string {
	return name
}

// creates a filter instance with the provided header key and value in the filter config.
func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
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
