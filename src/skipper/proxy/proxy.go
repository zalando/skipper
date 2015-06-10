package proxy

import "fmt"
import "io"
import "log"
import "net/http"
import "skipper/etcd"

const proxyBufferSize = 8192
const proxyErrorFmt = "proxy: %s"

type flusherWriter interface {
	http.Flusher
	io.Writer
}

type proxy struct {
	etcdc etcd.Etcdc
	tr   *http.Transport
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

func mapRequest(r *http.Request, s etcd.Settings) (*http.Request, error) {
	if s == nil {
		return nil, proxyError("missing settings")
	}

	if len(s.GetBackends()) == 0 {
		return nil, proxyError("missing backend settings")
	}

	b := s.GetBackends()["test"]
	if len(b.Servers) == 0 {
		return nil, proxyError("missing backend server settings")
	}

	return http.NewRequest(r.Method, b.Servers[0].Url, r.Body)
}

func MakeProxy(ec etcd.Etcdc) http.Handler {
	return &proxy{ec, &http.Transport{}}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		http.Error(w, http.StatusText(404), 404)
		log.Println(err)
	}

	rr, err := mapRequest(r, <-p.etcdc.GetSettings())
	if err != nil {
		hterr(err)
		return
	}

	rr.Header = cloneHeader(r.Header)

	rs, err := p.tr.RoundTrip(rr)
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
