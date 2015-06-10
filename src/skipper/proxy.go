package main

import "fmt"
import "io"
import "log"
import "net/http"

const proxyBufferSize = 8192
const proxyErrorFmt = "proxy: %s"

type flusherWriter interface {
	http.Flusher
	io.Writer
}

type proxy struct {
	etcd *etcdClient
    htclient *http.Client
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

		l, err := from.Read(b)
		if err != nil {
			return err
		}

		_, err = to.Write(b[:l])
		if err != nil {
			if err == io.EOF {
				return nil
			}

			return err
		}

		to.Flush()
	}
}

func mapRequest(r *http.Request, s settings) (*http.Request, error) {
	if s == nil {
		return nil, proxyError("missing settings")
	}

	if len(s.getBackends()) == 0 {
		return nil, proxyError("missing backend settings")
	}

	b := s.getBackends()[0]
	if len(b.servers) == 0 {
		return nil, proxyError("missing backend server settings")
	}

	return http.NewRequest(r.Method, b.servers[0].url, r.Body)
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hterr := func(err error) {
		http.Error(w, http.StatusText(404), 404)
		log.Println(err)
	}

	rr, err := mapRequest(r, <-p.etcd.getSettings())
	if err != nil {
		hterr(err)
		return
	}

	rr.Header = cloneHeader(r.Header)

	rs, err := p.htclient.Do(rr)
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
