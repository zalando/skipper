// Filter for http redirects. Accepts two arguments:
// a number as the redirect status code, and a string as the redirect location.
// This filter marks the request context served, and should be used only with shunt routes.
package redirect

import (
	"errors"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/url"
)

type Redirect struct {
	id       string
	code     int
	location *url.URL
}

func (spec *Redirect) Name() string { return "redirect" }

func (spec *Redirect) MakeFilter(id string, c skipper.FilterConfig) (skipper.Filter, error) {
	invalidArgs := func() (skipper.Filter, error) {
		return nil, errors.New("invalid arguments")
	}

	if len(c) != 2 {
		return invalidArgs()
	}

	code, ok := c[0].(float64)
	if !ok {
		return invalidArgs()
	}

	location, ok := c[1].(string)
	if !ok {
		return invalidArgs()
	}

	lu, err := url.Parse(location)
	if err != nil {
		return nil, err
	}

	return &Redirect{id, int(code), lu}, nil
}

func (f *Redirect) Id() string                        { return f.id }
func (f *Redirect) Request(ctx skipper.FilterContext) {}

func (f *Redirect) copyOfLocation() *url.URL {
	v := *f.location
	return &v
}

func getRequestHost(r *http.Request) string {
	h := r.Header.Get("Host")

	if h == "" {
		h = r.Host
	}

	if h == "" {
		h = r.URL.Host
	}

	return h
}

func (f *Redirect) Response(ctx skipper.FilterContext) {
	r := ctx.Request()
	w := ctx.ResponseWriter()
	u := f.copyOfLocation()

	if u.Scheme == "" {
		if r.URL.Scheme != "" {
			u.Scheme = r.URL.Scheme
		} else {
			u.Scheme = "https"
		}
	}

	u.User = r.URL.User

	if u.Host == "" {
		u.Host = getRequestHost(r)
	}

	if u.Path == "" {
		u.Path = r.URL.Path
	}

	if u.RawQuery == "" {
		u.RawQuery = r.URL.RawQuery
	}

	w.Header().Set("Location", u.String())
	w.WriteHeader(f.code)
	ctx.MarkServed()
}
