package proxy

import "fmt"
import "io"
import "log"
import "net/http"
import "skipper/settings"

const proxyBufferSize = 8192
const proxyErrorFmt = "proxy: %s"

type flusherWriter interface {
	http.Flusher
	io.Writer
}

type proxy struct {
	config    settings.Source
	transport *http.Transport
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
	for {
		b := make([]byte, proxyBufferSize)

		l, rerr := from.Read(b)
		if rerr != nil && rerr != io.EOF {
			return rerr
		}

		_, werr := to.Write(b[:l])
		if werr != nil {
			return werr
		}

		to.Flush()

		if rerr == io.EOF {
			return nil
		}
	}
}

func mapRequest(r *http.Request, s settings.Settings) (*http.Request, error) {
	if s == nil {
		return nil, proxyError("missing settings")
	}

	b, err := s.Route(r)
	if b == nil || err != nil {
		if err != nil {
			return nil, err
		}

		return nil, proxyError("route not found")
	}

	rr, err := http.NewRequest(r.Method, b.Url(), r.Body)
	if err != nil {
		return nil, err
	}

	rr.Header = cloneHeader(r.Header)
	return rr, nil
}

func Make(ss settings.Source) http.Handler {
	return &proxy{ss, &http.Transport{}}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		http.Error(w, http.StatusText(404), 404)
		log.Println(err)
	}

	rr, err := mapRequest(r, <-p.config.Get())
	if err != nil {
		hterr(err)
		return
	}

	rs, err := p.transport.RoundTrip(rr)
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

	copyHeader(w.Header(), rs.Header)
	w.WriteHeader(rs.StatusCode)
	copyStream(w.(flusherWriter), rs.Body)
}
