// a reverse proxy routing between frontends and backends based on the definitions in the received settings. on
// every request and response, it executes the filters if there are any defined for the given route.
package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/zalando/skipper/skipper"
	"io"
	"log"
	"net/http"
)

const (
	defaultSettingsBufferSize = 32
	proxyBufferSize           = 8192
	proxyErrorFmt             = "proxy: %s"

	// TODO: this should be fine tuned, yet, with benchmarks.
	// In case it doesn't make a big difference, then a lower value
	// can be safer, but the default 2 turned out to be low during
	// benchmarks.
	idleConnsPerHost = 64
)

type flusherWriter interface {
	http.Flusher
	io.Writer
}

type shuntBody struct {
	*bytes.Buffer
}

type proxy struct {
	settings     <-chan skipper.Settings
	roundTripper http.RoundTripper
}

type filterContext struct {
	w      http.ResponseWriter
	req    *http.Request
	res    *http.Response
	served bool
}

func (sb shuntBody) Close() error {
	return nil
}

func proxyError(m string) error {
	return fmt.Errorf(proxyErrorFmt, m)
}

func copyHeader(to, from http.Header) {
	for k, v := range from {
		to[http.CanonicalHeaderKey(k)] = v
	}
}

func cloneHeader(h http.Header) http.Header {
	hh := make(http.Header)
	copyHeader(hh, h)
	return hh
}

func copyStream(to flusherWriter, from io.Reader) error {
	b := make([]byte, proxyBufferSize)

	for {
		l, rerr := from.Read(b)
		if rerr != nil && rerr != io.EOF {
			return rerr
		}

		if l > 0 {
			_, werr := to.Write(b[:l])
			if werr != nil {
				return werr
			}

			to.Flush()
		}

		if rerr == io.EOF {
			return nil
		}
	}
}

func mapRequest(r *http.Request, b skipper.Backend) (*http.Request, error) {
	if b == nil {
		return nil, proxyError("missing backend")
	}

	u := r.URL
	u.Scheme = b.Scheme()
	u.Host = b.Host()

	rr, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}

	rr.Header = cloneHeader(r.Header)
	return rr, nil
}

func getSettingsBufferSize() int {
	// todo: return defaultFeedBufferSize when not dev env
	return 0
}

// creates a proxy. it expects a settings source, that provides the current settings during each request.
// if the 'insecure' parameter is true, the proxy skips the TLS verification for the requests made to the
// backends.
func Make(sd skipper.SettingsSource, insecure bool) http.Handler {
	sc := make(chan skipper.Settings, getSettingsBufferSize())
	sd.Subscribe(sc)

	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &proxy{sc, tr}
}

func applyFilterSafe(id string, p func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("filter", id, err)
		}
	}()

	p()
}

func applyFiltersToRequest(f []skipper.Filter, ctx skipper.FilterContext) {
	for _, fi := range f {
		applyFilterSafe(fi.Id(), func() {
			fi.Request(ctx)
		})
	}
}

func applyFiltersToResponse(f []skipper.Filter, ctx skipper.FilterContext) {
	for i, _ := range f {
		fi := f[len(f)-1-i]
		applyFilterSafe(fi.Id(), func() {
			fi.Response(ctx)
		})
	}
}

func (c *filterContext) ResponseWriter() http.ResponseWriter {
	return c.w
}

func (c *filterContext) Request() *http.Request {
	return c.req
}

func (c *filterContext) Response() *http.Response {
	return c.res
}

func (c *filterContext) MarkServed() {
	c.served = true
}

func (c *filterContext) IsServed() bool {
	return c.served
}

func shunt(r *http.Request) *http.Response {
	return &http.Response{
		StatusCode: 404,
		Header:     make(http.Header),
		Body:       &shuntBody{&bytes.Buffer{}},
		Request:    r}
}

func (p *proxy) roundtrip(r *http.Request, b skipper.Backend) (*http.Response, error) {
	rr, err := mapRequest(r, b)
	if err != nil {
		return nil, err
	}

	return p.roundTripper.RoundTrip(rr)
}

// http.Handler implementation
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		// todo: just a bet that we shouldn't send here 50x
		http.Error(w, http.StatusText(404), 404)
		log.Println(err)
	}

	s := <-p.settings
	if s == nil {
		hterr(proxyError("missing settings"))
		return
	}

	rt, err := s.Route(r)
	if rt == nil || err != nil {
		// todo: we need a somewhat more extensive logging here
		hterr(proxyError(fmt.Sprintf("routing failed: %v %v", r.URL, err)))
		return
	}

	f := rt.Filters()
	c := &filterContext{w, r, nil, false}
	applyFiltersToRequest(f, c)

	b := rt.Backend()
	var rs *http.Response
	if b.IsShunt() {
		rs = shunt(r)
	} else {
		rs, err = p.roundtrip(r, rt.Backend())
		if err != nil {
			hterr(err)
			return
		}

		defer func() {
			err = rs.Body.Close()
			if err != nil {
				log.Println(err)
			}
		}()
	}

	c.res = rs
	applyFiltersToResponse(f, c)

	if !c.IsServed() {
		copyHeader(w.Header(), rs.Header)
		w.WriteHeader(rs.StatusCode)
		copyStream(w.(flusherWriter), rs.Body)
	}
}
