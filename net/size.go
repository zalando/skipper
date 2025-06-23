package net

import (
	"context"
	"net/http"
)

// SizeOfRequestHeader returns size of the HTTP request header
func SizeOfRequestHeader(r *http.Request) (size int) {
	return len(valueOrDefault(r.Method, "GET")) + len(" ") + len(r.URL.RequestURI()) + len(" HTTP/1.1\r\n") +
		len("Host: ") + len(valueOrDefault(r.Host, r.URL.Host)) + len("\r\n") +
		sizeOfHeader(r.Header) + len("\r\n")
}

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func sizeOfHeader(h http.Header) (size int) {
	for k, vv := range h {
		for _, v := range vv {
			size += len(k) + len(": ") + len(v) + len("\r\n")
		}
	}
	return
}

func exactSizeOfRequestHeader(r *http.Request) (size int) {
	r = r.Clone(context.Background())
	r.Body = nil // discard body
	w := &countingWriter{}
	r.Write(w)
	return w.size
}

type countingWriter struct {
	size int
}

func (w *countingWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	w.size += n
	return n, nil
}
