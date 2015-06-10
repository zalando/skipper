package main

import "io"
import "log"
import "net/http"

const bufferSize = 8192;

type flusherWriter interface {
    http.Flusher
    io.Writer
}

type proxy struct {}

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
        b := make([]byte, bufferSize)

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
