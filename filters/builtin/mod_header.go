package builtin

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/zalando/skipper/filters"
)

type modRequestHeader struct {
	headerName  string
	rx          *regexp.Regexp
	replacement string
}

// NewModRequestHeader returns a new filter Spec, whose instances execute
// regexp.ReplaceAllString on the request host. Instances expect three
// parameters: the header name, the expression to match and the replacement string.
// Name: "modRequestHeader".
func NewModRequestHeader() filters.Spec { return &modRequestHeader{} }

func (spec *modRequestHeader) Name() string {
	return filters.ModRequestHeaderName
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *modRequestHeader) CreateFilter(config []any) (filters.Filter, error) {
	if len(config) != 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	expr, ok := config[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	replacement, ok := config[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	rx, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	return &modRequestHeader{headerName: headerName, rx: rx, replacement: replacement}, nil
}

func (f *modRequestHeader) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	if strings.ToLower(f.headerName) == "host" {
		nh := f.rx.ReplaceAllString(getRequestHost(req), f.replacement)

		req.Header.Set(f.headerName, nh)
		ctx.SetOutgoingHost(nh)

		return
	}

	if _, ok := req.Header[http.CanonicalHeaderKey(f.headerName)]; !ok {
		return
	}

	req.Header.Set(f.headerName, f.rx.ReplaceAllString(req.Header.Get(f.headerName), f.replacement))
}

func (*modRequestHeader) Response(filters.FilterContext) {}

type modResponseHeader struct {
	headerName  string
	rx          *regexp.Regexp
	replacement string
}

// NewModResponseHeader returns a new filter Spec, whose instances execute
// regexp.ReplaceAllString on the request host. Instances expect three
// parameters: the header name, the expression to match and the replacement string.
// Name: "modResponseHeader".
func NewModResponseHeader() filters.Spec { return &modResponseHeader{} }

func (spec *modResponseHeader) Name() string {
	return filters.ModResponseHeaderName
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *modResponseHeader) CreateFilter(config []any) (filters.Filter, error) {
	if len(config) != 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	expr, ok := config[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	replacement, ok := config[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	rx, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	return &modResponseHeader{headerName: headerName, rx: rx, replacement: replacement}, nil
}

func (*modResponseHeader) Request(filters.FilterContext) {}

func (f *modResponseHeader) Response(ctx filters.FilterContext) {
	resp := ctx.Response()

	if _, ok := resp.Header[http.CanonicalHeaderKey(f.headerName)]; !ok {
		return
	}

	resp.Header.Set(f.headerName, f.rx.ReplaceAllString(resp.Header.Get(f.headerName), f.replacement))
}
