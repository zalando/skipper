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
	// statusPrefixes selects which responses get logged, following the
	// same matching rules as the enableAccessLog filter: a value below 10
	// matches a status class (5 matches 5xx), below 100 a sub-class (50
	// matches 50x) and any larger value an exact status code. Empty means
	// no condition, i.e. always log.
	statusPrefixes []int
}

// NewLogBody creates a filter specification for the 'logBody()' filter.
func NewLogBody() filters.Spec { return logBody{} }

// Name returns the logBody filter name.
func (logBody) Name() string {
	return filters.LogBodyName
}

func (logBody) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		request  = false
		response = false
	)

	if len(args) < 2 {
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

	var statusPrefixes []int
	for _, arg := range args[2:] {
		prefix, ok := arg.(float64)
		if !ok || float64(int(prefix)) != prefix {
			return nil, fmt.Errorf("failed to convert to int: %w", filters.ErrInvalidFilterParameters)
		}
		p := int(prefix)
		if !validStatusPrefix(p) {
			return nil, fmt.Errorf("status prefix %d cannot match a response status: %w", p, filters.ErrInvalidFilterParameters)
		}
		statusPrefixes = append(statusPrefixes, p)
	}

	return &logBody{
		limit:          int(limit),
		request:        request,
		response:       response,
		statusPrefixes: statusPrefixes,
	}, nil
}

// validStatusPrefix reports whether prefix can select any response status.
// A prefix below 10 matches a status class, below 100 a sub-class and
// anything larger an exact code, so only the values that reach into the
// [100, 599] range are accepted. This rejects typos such as 0, 6 or 600,
// which would otherwise silently never log.
func validStatusPrefix(prefix int) bool {
	switch {
	case prefix < 10:
		return prefix >= 1 && prefix <= 5
	case prefix < 100:
		return prefix >= 10 && prefix <= 59
	default:
		return prefix <= 599
	}
}

// matchStatus reports whether statusCode is selected by the configured
// prefixes. The rules are the same as the enableAccessLog filter's.
func (lb *logBody) matchStatus(statusCode int) bool {
	for _, prefix := range lb.statusPrefixes {
		switch {
		case prefix < 10:
			if statusCode >= prefix*100 && statusCode < (prefix+1)*100 {
				return true
			}
		case prefix < 100:
			if statusCode >= prefix*10 && statusCode < (prefix+1)*10 {
				return true
			}
		default:
			if statusCode == prefix {
				return true
			}
		}
	}
	return false
}

func (lb *logBody) Request(ctx filters.FilterContext) {
	if !lb.request {
		return
	}

	req := ctx.Request()
	if req.Body == nil {
		return
	}

	if len(lb.statusPrefixes) == 0 {
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
	if len(lb.statusPrefixes) > 0 && !lb.matchStatus(ctx.Response().StatusCode) {
		return
	}

	if lb.request {
		if len(lb.statusPrefixes) > 0 {
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
