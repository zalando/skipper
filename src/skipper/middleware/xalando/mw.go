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
		println("has cookie")
		uuid = c.Value
	}

	if len(uuid) == 0 {
		println("trying to create new uuid")
		u, err := gouuid.NewV4()
		if err == nil {
			println("created new uuid")
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
		println("setting uuid in the header")
		ctx.Request().Header["X-Zalando-Session-Id"] = []string{uuid}
	}
}

func (mw *impl) Request(ctx skipper.FilterContext) {
	println("executing filter")
	req := ctx.Request()
	req.Header["X-Zalando-Request-Host"] = []string{req.Host}
	req.Header["X-Zalando-Request-URI"] = []string{req.RequestURI}
	extractOrGenerateSessionId(ctx)
}

// todo: cleanup these weak experiments with mimicked inheritance
func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	f := &impl{&noop.Type{}}
	noop.InitFilter(f.Type, id)
	return f, nil
}
