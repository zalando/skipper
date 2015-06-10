package main

import "io"
import "log"
import "net/http"

type proxy struct {
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

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    hterr := func (err error) {
        http.Error(w, http.StatusText(404), 404)
        log.Println(err)
    }

    rr, err := http.NewRequest("GET", "http://localhost:9999/slow", r.Body)
    if err != nil {
        hterr(err)
    }

    rr.Header = cloneHeader(r.Header)

    c := &http.Client{}
    rs, err := c.Do(rr)
    if err != nil {
        hterr(err)
    }

    defer func() {
        err = rs.Body.Close()
        if err != nil {
            log.Println(err)
        }
    }()

    copyHeader(w.Header(), rs.Header)
    w.WriteHeader(rs.StatusCode)
    io.Copy(w, rs.Body)
}
