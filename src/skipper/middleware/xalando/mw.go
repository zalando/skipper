// zalando fashion store specific middleware
//
// It sets the following headers:
// X-Zalando-Session-Id,
// X-Zalando-Request-Host,
// X-Zalando-Request-URI.
//
// It sets the
// Zalando-Session-Id cookie.
//
// session id:
//
// if the session id cookie is received in the request, it copies to a header field called X-Zalando-Session-Id.
// if the request doesn't contain a session id cookie, it generates one, sets it in both the response writer and
// request header.
//
// host and request uri:
//
// sets both the host and request uri in a header field, so that the proxied hosts receive the original values.
package xalando

import (
	"net/http"
	"skipper/middleware/noop"
	"skipper/skipper"

	gouuid "github.com/nu7hatch/gouuid"
)

const name = "xalando"

type impl struct {
	*noop.Type
}

// creates a middleware instance
func Make() *impl {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}

func extractOrGenerateSessionId(ctx skipper.FilterContext) {
	var uuid string

	c, err := ctx.Request().Cookie("Zalando-Session-Id")
	if err == nil {
		uuid = c.Value
	}

	if len(uuid) == 0 {
		u, err := gouuid.NewV4()
		if err == nil {
			uuid = u.String()
			http.SetCookie(ctx.ResponseWriter(), &http.Cookie{
				Name:   "Zalando-Session-Id",
				Value:  uuid,
				Path:   "/",
				Domain: "zalan.do",
				MaxAge: 2147483647})
		}
	}

	if len(uuid) != 0 {
		ctx.Request().Header["X-Zalando-Session-Id"] = []string{uuid}
	}
}

// processes the request
func (mw *impl) Request(ctx skipper.FilterContext) {
	req := ctx.Request()
	req.Header["X-Zalando-Request-Host"] = []string{req.Host}
	req.Header["X-Zalando-Request-URI"] = []string{req.RequestURI}
	extractOrGenerateSessionId(ctx)
}

// creates a filter instance
func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	f := &impl{&noop.Type{}}
	f.SetId(id)
	return f, nil
}
