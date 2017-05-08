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
	"time"

	"github.com/zalando/skipper/filters"
)

const (
	RequestCookieFilterName    = "requestCookie"
	ResponseCookieFilterName   = "responseCookie"
	ResponseJSCookieFilterName = "jsCookie"
	ChangeOnlyArg              = "change-only"
	SetCookieHttpHeader        = "Set-Cookie"
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
	ttl        time.Duration
	changeOnly bool
}

// Creates a filter spec for appending cookies to requests.
// Name: requestCookie
func NewRequestCookie() filters.Spec {
	return &spec{request, RequestCookieFilterName}
}

// Creates a filter spec for appending cookies to responses.
// Name: responseCookie
func NewResponseCookie() filters.Spec {
	return &spec{response, ResponseCookieFilterName}
}

// Creates a filter spec for appending cookies to responses without the
// HttpOnly directive.
// Name: jsCookie
func NewJSCookie() filters.Spec {
	return &spec{responseJS, ResponseJSCookieFilterName}
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
		if ttl, ok := args[2].(float64); ok {
			f.ttl = time.Duration(ttl) * time.Second
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
	var set func(filters.FilterContext, string, string, time.Duration)
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
		set(ctx, f.name, f.value, f.ttl)
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

	set(ctx, f.name, f.value, f.ttl)
}

func setCookie(ctx filters.FilterContext, name, value string, ttl time.Duration, jsEnabled bool) {
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
		MaxAge:   int(ttl.Seconds())}

	ctx.Response().Header.Add(SetCookieHttpHeader, c.String())
}

func configSetCookie(jscookie bool) func(filters.FilterContext, string, string, time.Duration) {
	return func(ctx filters.FilterContext, name, value string, ttl time.Duration) {
		setCookie(ctx, name, value, ttl, jscookie)
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
