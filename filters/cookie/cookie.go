/*
Package cookie implements filters to append to requests or responses.

It implements two filters, one for appending cookies to requests in
the "Cookie" header, and one for appending cookies to responses in the
"Set-Cookie" header.

Both the request and response cookies expect a name and a value argument.

The response cookie accepts an optional argument to control the max-age
property of the cookie, of type number, in seconds.

The response cookie accepts an optional fourth argument, "change-only",
to control if the cookie should be set on every response, or only if the
request doesn't contain a cookie with the provided name and value. If the
fourth argument is "change-only", and a cookie with the same name and value
is found in the request, the cookie is not set. This argument can be used
to disable sliding TTL of the cookie.

The JS cookie behaves exactly as the response cookie, but it doesn't
set the HttpOnly directive, so these cookies will be
accessible from JS code running in web browsers.

Examples:

	requestCookie("test-session", "abc")

	responseCookie("test-session", "abc", 31536000)

	responseCookie("test-session", "abc", 31536000, "change-only")

	// response cookie without HttpOnly:
	jsCookie("test-session-info", "abc-debug", 31536000, "change-only")
*/
package cookie

import (
	"net"
	"net/http"
	"strings"

	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.RequestCookieName instead
	RequestCookieFilterName = filters.RequestCookieName
	// Deprecated, use filters.ResponseCookieName instead
	ResponseCookieFilterName = filters.ResponseCookieName
	// Deprecated, use filters.JsCookieName instead
	ResponseJSCookieFilterName = filters.JsCookieName

	ChangeOnlyArg       = "change-only"
	SetCookieHttpHeader = "Set-Cookie"
)

type direction int

const (
	request direction = iota
	response
	responseJS
)

type spec struct {
	typ        direction
	filterName string
}

type filter struct {
	typ        direction
	name       string
	value      string
	maxAge     int
	changeOnly bool
}

type dropCookie struct {
	typ  direction
	name string
}

func NewDropRequestCookie() filters.Spec {
	return &dropCookie{
		typ: request,
	}
}

func NewDropResponseCookie() filters.Spec {
	return &dropCookie{
		typ: response,
	}
}

func (d *dropCookie) Name() string {
	switch d.typ {
	case request:
		return filters.DropRequestCookieName
	case response:
		return filters.DropResponseCookieName
	}
	return "unknown"
}

func (d *dropCookie) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	s, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &dropCookie{
		typ:  d.typ,
		name: s,
	}, nil
}

func removeCookie(request *http.Request, name string) bool {
	cookies := request.Cookies()
	hasCookie := false
	for _, c := range cookies {
		if c.Name == name {
			hasCookie = true
			break
		}
	}

	if hasCookie {
		request.Header.Del("Cookie")
		for _, c := range cookies {
			if c.Name != name {
				request.AddCookie(c)
			}
		}
	}
	return hasCookie
}

func removeCookieResponse(rsp *http.Response, name string) bool {
	cookies := rsp.Cookies()
	hasCookie := false
	for _, c := range cookies {
		if c.Name == name {
			hasCookie = true
			break
		}
	}

	if hasCookie {
		rsp.Header.Del("Set-Cookie")
		for _, c := range cookies {
			if c.Name != name {
				rsp.Header.Add("Set-Cookie", c.String())
			}
		}
	}
	return hasCookie
}

func (d *dropCookie) Request(ctx filters.FilterContext) {
	if d.typ != request {
		return
	}
	removeCookie(ctx.Request(), d.name)
}

func (d *dropCookie) Response(ctx filters.FilterContext) {
	if d.typ != response {
		return
	}
	removeCookieResponse(ctx.Response(), d.name)
}

// NewRequestCookie creates a filter spec for appending cookies to requests.
// Name: requestCookie
func NewRequestCookie() filters.Spec {
	return &spec{request, filters.RequestCookieName}
}

// NewResponseCookie creates a filter spec for appending cookies to responses.
// Name: responseCookie
func NewResponseCookie() filters.Spec {
	return &spec{response, filters.ResponseCookieName}
}

// NewJSCookie creates a filter spec for appending cookies to responses without the
// HttpOnly directive.
// Name: jsCookie
func NewJSCookie() filters.Spec {
	return &spec{responseJS, filters.JsCookieName}
}

func (s *spec) Name() string { return s.filterName }

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 2 || (len(args) > 2 && s.typ == request) || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &filter{typ: s.typ}

	if name, ok := args[0].(string); ok && name != "" {
		f.name = name
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}

	if value, ok := args[1].(string); ok {
		f.value = value
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(args) >= 3 {
		if maxAge, ok := args[2].(float64); ok {
			// https://pkg.go.dev/net/http#Cookie uses zero to omit Max-Age attribute:
			// > MaxAge=0 means no 'Max-Age' attribute specified.
			// > MaxAge<0 means delete cookie now, equivalently 'Max-Age: 0'
			// > MaxAge>0 means Max-Age attribute present and given in seconds
			//
			// Here we know user specified Max-Age explicitly, so we interpret zero
			// as a signal to delete the cookie similar to what user would expect naturally,
			// see https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie#max-agenumber
			// > A zero or negative number will expire the cookie immediately.
			if maxAge == 0 {
				f.maxAge = -1
			} else {
				f.maxAge = int(maxAge)
			}
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	if len(args) == 4 {
		f.changeOnly = args[3] == ChangeOnlyArg
	}

	return f, nil
}

func (f *filter) Request(ctx filters.FilterContext) {
	if f.typ != request {
		return
	}

	ctx.StateBag()["CookieSet:"+f.name] = f.value

	ctx.Request().AddCookie(&http.Cookie{Name: f.name, Value: f.value})
}

func (f *filter) Response(ctx filters.FilterContext) {
	var set func(filters.FilterContext, string, string, int)
	switch f.typ {
	case request:
		return
	case response:
		set = configSetCookie(false)
	case responseJS:
		set = configSetCookie(true)
	default:
		panic("invalid cookie filter type")
	}

	ctx.StateBag()["CookieSet:"+f.name] = f.value

	if !f.changeOnly {
		set(ctx, f.name, f.value, f.maxAge)
		return
	}

	var req *http.Request
	if req = ctx.OriginalRequest(); req == nil {
		req = ctx.Request()
	}

	requestCookie, err := req.Cookie(f.name)
	if err == nil && requestCookie.Value == f.value {
		return
	}

	set(ctx, f.name, f.value, f.maxAge)
}

func setCookie(ctx filters.FilterContext, name, value string, maxAge int, jsEnabled bool) {
	var req = ctx.Request()
	if ctx.OriginalRequest() != nil {
		req = ctx.OriginalRequest()
	}
	d := extractDomainFromHost(req.Host)
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: !jsEnabled,
		Secure:   true,
		Domain:   d,
		Path:     "/",
		MaxAge:   maxAge,
	}

	ctx.Response().Header.Add(SetCookieHttpHeader, c.String())
}

func configSetCookie(jscookie bool) func(filters.FilterContext, string, string, int) {
	return func(ctx filters.FilterContext, name, value string, maxAge int) {
		setCookie(ctx, name, value, maxAge, jscookie)
	}
}

func extractDomainFromHost(host string) string {
	h, _, err := net.SplitHostPort(host)

	if err != nil {
		h = host
	}

	if strings.Count(h, ".") < 2 {
		return h
	}

	return strings.Join(strings.Split(h, ".")[1:], ".")
}
