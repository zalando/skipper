package builtin

import (
	"github.com/zalando/skipper/filters"
)

type (
	cookieToHeaderSpec struct {
	}

	cookieToHeaderFilter struct {
		cookieName string
		headerName string
		prefix     string
	}
)

// NewCookieToHeader creates a filter which copies a cookie value
// from the incoming Request to headers, with an optional prefix
//
// 		cookieToHeader("jwt", "Authorization", "Bearer ")
//
// The above filter will set the value of the Authorization header in the request to
// 'Bearer <contents of the "jwt" cookie>'
func NewCookieToHeader() filters.Spec {
	return &cookieToHeaderSpec{}
}

func (spec *cookieToHeaderSpec) Name() string {
	return CookieToHeaderName
}

func (spec *cookieToHeaderSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	switch len(args) {
	case 2, 3:
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	cookieName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	prefix := ""
	if len(args) >= 3 {
		prefix, ok = args[2].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &cookieToHeaderFilter{
		cookieName: cookieName,
		headerName: headerName,
		prefix:     prefix,
	}, nil
}

func (f *cookieToHeaderFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	if cookie, err := req.Cookie(f.cookieName); err == nil {
		req.Header.Set(f.headerName, f.prefix+cookie.Value)
	}
}

func (f *cookieToHeaderFilter) Response(ctx filters.FilterContext) {}
