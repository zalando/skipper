package diag

import (
	"fmt"
	"io"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
)

type logBody struct {
	limit    int
	request  bool
	response bool
}

// NewLogBody creates a filter specification for the 'logBody()' filter.
func NewLogBody() filters.Spec { return logBody{} }

// Name returns the logBody filter name.
func (logBody) Name() string {
	return filters.LogBodyName
}

func (logBody) CreateFilter(args []any) (filters.Filter, error) {
	var (
		request  = false
		response = false
	)

	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	opt, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	switch opt {
	case "response":
		response = true
	case "request":
		request = true
	default:
		return nil, fmt.Errorf("failed to match %q: %w", opt, filters.ErrInvalidFilterParameters)
	}

	limit, ok := args[1].(float64)
	if !ok || float64(int(limit)) != limit {
		return nil, fmt.Errorf("failed to convert to int: %w", filters.ErrInvalidFilterParameters)
	}

	return &logBody{
		limit:    int(limit),
		request:  request,
		response: response,
	}, nil
}

func (lb *logBody) Request(ctx filters.FilterContext) {
	if !lb.request {
		return
	}

	req := ctx.Request()
	if req.Body != nil {
		req.Body = newLogBodyStream(
			lb.limit,
			func(chunk []byte) {
				ctx.Logger().Infof(
					`logBody("request") %s: %q`,
					req.Header.Get(flowid.HeaderName),
					chunk)
			},
			req.Body,
		)
	}
}

func (lb *logBody) Response(ctx filters.FilterContext) {
	if !lb.response {
		return
	}

	rsp := ctx.Response()
	if rsp.Body != nil {
		rsp.Body = newLogBodyStream(
			lb.limit,
			func(chunk []byte) {
				ctx.Logger().Infof(
					`logBody("response") %s: %q`,
					ctx.Request().Header.Get(flowid.HeaderName),
					chunk)
			},
			rsp.Body,
		)
	}
}

type logBodyStream struct {
	left  int
	f     func([]byte)
	input io.ReadCloser
}

func newLogBodyStream(left int, f func([]byte), rc io.ReadCloser) io.ReadCloser {
	return &logBodyStream{
		left:  left,
		f:     f,
		input: rc,
	}
}

func (lb *logBodyStream) Read(p []byte) (n int, err error) {
	n, err = lb.input.Read(p)
	if lb.left > 0 && n > 0 {
		m := min(n, lb.left)
		lb.f(p[:m])
		lb.left -= m
	}
	return n, err
}

func (lb *logBodyStream) Close() error {
	return lb.input.Close()
}
