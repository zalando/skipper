package diag

import (
	"bytes"
	"fmt"
	"io"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
)

// logBodyRequestKey is the state bag key used to hand the captured request
// body from the request to the response phase, when a status condition
// defers the decision to log until the response status is known.
const logBodyRequestKey = "filter." + filters.LogBodyName + ".request"

type logBody struct {
	limit    int
	request  bool
	response bool
	// minStatus is the response status code from which on the body is
	// logged. Zero means no condition, i.e. always log.
	minStatus int
}

// NewLogBody creates a filter specification for the 'logBody()' filter.
//
// It takes the type of the body to log, "request" or "response", a limit
// of the number of bytes to log and an optional response status code from
// which on to log. Without the status code the body is logged in chunks
// while it streams. With it, a response body is logged the same way once
// the status matches, while a request body is buffered up to the limit
// and logged after the response status is known.
func NewLogBody() filters.Spec { return logBody{} }

// Name returns the logBody filter name.
func (logBody) Name() string {
	return filters.LogBodyName
}

func (logBody) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		request   = false
		response  = false
		minStatus = 0
	)

	if len(args) != 2 && len(args) != 3 {
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

	if len(args) == 3 {
		status, ok := args[2].(float64)
		if !ok || float64(int(status)) != status {
			return nil, fmt.Errorf("failed to convert to int: %w", filters.ErrInvalidFilterParameters)
		}
		minStatus = int(status)
		if minStatus < 100 || minStatus > 599 {
			return nil, fmt.Errorf("status %d out of range [100, 599]: %w", minStatus, filters.ErrInvalidFilterParameters)
		}
	}

	return &logBody{
		limit:     int(limit),
		request:   request,
		response:  response,
		minStatus: minStatus,
	}, nil
}

func (lb *logBody) Request(ctx filters.FilterContext) {
	if !lb.request {
		return
	}

	req := ctx.Request()
	if req.Body == nil {
		return
	}

	if lb.minStatus == 0 {
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
		return
	}

	// The response status is not known yet, so buffer instead of logging.
	// The limit caps the buffer, the request body itself is not held back.
	buf := &bytes.Buffer{}
	ctx.StateBag()[logBodyRequestKey] = buf
	req.Body = newLogBodyStream(
		lb.limit,
		func(chunk []byte) { buf.Write(chunk) },
		req.Body,
	)
}

func (lb *logBody) Response(ctx filters.FilterContext) {
	if lb.minStatus > 0 && ctx.Response().StatusCode < lb.minStatus {
		return
	}

	if lb.request {
		if lb.minStatus > 0 {
			lb.logBufferedRequest(ctx)
		}
		return
	}

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

func (lb *logBody) logBufferedRequest(ctx filters.FilterContext) {
	buf, ok := ctx.StateBag()[logBodyRequestKey].(*bytes.Buffer)
	if !ok || buf.Len() == 0 {
		return
	}

	ctx.Logger().Infof(
		`logBody("request") %s: %q`,
		ctx.Request().Header.Get(flowid.HeaderName),
		buf.Bytes())
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
