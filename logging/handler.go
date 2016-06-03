package logging

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"
)

// The logging handler wraps the proxy handler to produce an access log compatible to Apache's
type loggingHandler struct {
	proxy http.Handler
}

// Creates an http.Handler that provides access log
// for the underlying handler.
func NewHandler(next http.Handler) http.Handler {
	return &loggingHandler{proxy: next}
}

func (lh *loggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	lw := &loggingWriter{writer: w}
	lh.proxy.ServeHTTP(lw, r)

	dur := time.Now().Sub(now)

	entry := &AccessEntry{
		Request:      r,
		ResponseSize: lw.bytes,
		StatusCode:   lw.code,
		RequestTime:  now,
		Duration:     dur,
	}
	LogAccess(entry)
}

func (lh *loggingHandler) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hij, ok := lh.proxy.(http.Hijacker)
	if ok {
		return hij.Hijack()
	}
	fmt.Printf("Could not Hijack logging handler")
	panic("foo")
}
