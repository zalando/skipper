package diag

import (
	"context"
	"strings"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
)

type logBody struct {
	request  bool
	response bool
}

// NewLogBody creates a filter specification for the 'logBody()' filter.
func NewLogBody() filters.Spec { return logBody{} }

// Name returns the logBody filtern name.
func (logBody) Name() string {
	return filters.LogBodyName
}

func (logBody) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		request  = false
		response = false
	)

	// default behavior
	if len(args) == 0 {
		request = true
	}

	for i := range args {
		opt, ok := args[i].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		switch strings.ToLower(opt) {
		case "response":
			response = true
		case "request":
			request = true
		}

	}

	return logBody{
		request:  request,
		response: response,
	}, nil
}

func (lb logBody) Request(ctx filters.FilterContext) {
	if !lb.request {
		return
	}

	req := ctx.Request()
	if req.Body != nil {
		req.Body = net.WrapBody(
			req.Context(),
			func(p []byte) (int, error) {
				ctx.Logger().Infof(`logBody("request"): %q`, p)
				return len(p), nil
			},
			req.Body)
	}
}

func (lb logBody) Response(ctx filters.FilterContext) {
	if !lb.response {
		return
	}

	rsp := ctx.Response()
	if rsp.Body != nil {
		// if this is not set we get from curl
		//    Error while processing content unencoding: invalid stored block lengths
		rsp.Header.Del("Content-Length")
		rsp.ContentLength = -1

		rsp.Body = net.WrapBody(
			context.Background(), // not sure if it makes sense to be cancellable here
			func(p []byte) (int, error) {
				ctx.Logger().Infof(`logBody("response"): %q`, p)
				return len(p), nil
			}, rsp.Body)
	}
}
