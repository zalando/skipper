// Filter for http redirects. Accepts two arguments:
// a number as the redirect status code, and a string as the redirect location.
// This filter marks the request context served, and should be used only with shunt routes.
package filters

import (
	"errors"
	"net/http"
	"net/url"
)

type Redirect struct {
	id       string
	code     int
	location string
}

func (spec *Redirect) Name() string { return "redirect" }

func (spec *Redirect) MakeFilter(id string, config []interface{}) (Filter, error) {
	invalidArgs := func() (Filter, error) {
		return nil, errors.New("invalid arguments")
	}

	if len(config) != 2 {
		return invalidArgs()
	}

	code, ok := config[0].(float64)
	if !ok {
		return invalidArgs()
	}

	location, ok := config[1].(string)
	if !ok {
		return invalidArgs()
	}

	return &Redirect{id, int(code), location}, nil
}

func (f *Redirect) Id() string                { return f.id }
func (f *Redirect) Request(ctx FilterContext) {}

func (f *Redirect) Response(ctx FilterContext) {
	w := ctx.ResponseWriter()

	u, err := url.Parse(f.location)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		ctx.MarkServed()
		return
	}

	if u.Host == "" {
		u.Scheme = ctx.Request().URL.Scheme
		u.Host = ctx.Request().URL.Host
	}

	w.Header().Set("Location", u.String())
	w.WriteHeader(f.code)
	ctx.MarkServed()
}
