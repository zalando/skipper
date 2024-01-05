package diag

import (
	"context"
	"fmt"
	"strings"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/io"
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
		req.Body = io.LogBody(
			req.Context(),
			fmt.Sprintf(`logBody("request") %s: `, req.Header.Get(flowid.HeaderName)),
			ctx.Logger().Infof,
			req.Body,
		)
	}
}

func (lb logBody) Response(ctx filters.FilterContext) {
	if !lb.response {
		return
	}

	rsp := ctx.Response()
	if rsp.Body != nil {
		rsp.Body = io.LogBody(
			context.Background(),
			fmt.Sprintf(`logBody("response") %s: `, ctx.Request().Header.Get(flowid.HeaderName)),
			ctx.Logger().Infof,
			rsp.Body,
		)
	}
}
