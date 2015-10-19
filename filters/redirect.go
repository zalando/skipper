// Filter for http redirects. Accepts two arguments:
// a number as the redirect status code, and a string as the redirect location.
// This filter marks the request context served, and should be used only with shunt routes.
package filters

import (
	"errors"
	"net/url"
    "net/http"
)

const RedirectName = "redirect"

type Redirect struct {
	code     int
	location *url.URL
}

func (spec *Redirect) Name() string { return RedirectName }

func (spec *Redirect) CreateFilter(config []interface{}) (Filter, error) {
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

	u, err := url.Parse(location)
	if err != nil {
		return invalidArgs()
	}

	return &Redirect{int(code), u}, nil
}

func (f *Redirect) Request(ctx FilterContext) {}

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

func (f *Redirect) Response(ctx FilterContext) {
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
