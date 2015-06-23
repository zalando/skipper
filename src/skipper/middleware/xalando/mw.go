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

func Make() *impl {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}

func extractOrGenerateSessionId(ctx skipper.FilterContext) {
	var uuid string

	c, err := ctx.Request().Cookie("X-Zalando-Session-Id")
	if err == nil {
		uuid = c.Value
	}

	if len(uuid) == 0 {
		u, err := gouuid.NewV4()
		if err == nil {
			uuid = u.String()
			http.SetCookie(ctx.ResponseWriter(), &http.Cookie{
				Name:   "X-Zalando-Session-Id",
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

func (mw *impl) Request(ctx skipper.FilterContext) {
	req := ctx.Request()
	req.Header["X-Zalando-Request-Host"] = []string{req.Host}
	req.Header["X-Zalando-Request-URI"] = []string{req.RequestURI}
	extractOrGenerateSessionId(ctx)
}

func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	f := &impl{&noop.Type{}}
	f.SetId(id)
	return f, nil
}
