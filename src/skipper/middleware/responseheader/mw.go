package responseheader

import (
	"net/http"
	"skipper/middleware/simpleheader"
	"skipper/skipper"
)

const name = "response-header"

type impl struct {
	simpleheader.Type
}

func Make() skipper.Middleware {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}

func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	f := &impl{}
	err := f.InitFilter(id, config)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (f *impl) ProcessResponse(r *http.Response) *http.Response {
	r.Header.Add(f.Key(), f.Value())
	return r
}
