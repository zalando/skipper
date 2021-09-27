package routesrv

import (
	"bytes"
	"net/http"
	"sync"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/tracing"
)

// eskipBytes keeps eskip-formatted routes as a byte slice and
// provides synchronized r/w access to them. Additionally it can
// serve as an HTTP handler exposing its content.
type eskipBytes struct {
	data        []byte
	initialized bool
	mu          sync.RWMutex

	tracer ot.Tracer
}

// bytes returns a slice to stored bytes, which are safe for reading,
// and if there were already initialized.
func (e *eskipBytes) bytes() ([]byte, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.data, e.initialized
}

// formatAndSet takes a slice of routes and stores them eskip-formatted
// in a synchronized way. It returns a number of stored bytes and a boolean,
// being true, when the stored bytes were set for the first time.
func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (int, bool) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}, routes...)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.data = buf.Bytes()
	oldInitialized := e.initialized
	e.initialized = true

	return len(e.data), !oldInitialized
}

func (e *eskipBytes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := tracing.CreateSpan("serve_routes", r.Context(), e.tracer)
	defer span.Finish()

	if data, initialized := e.bytes(); initialized {
		w.Write(data)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

// eskipBytesStatus serves as an HTTP health check for the referenced eskipBytes.
// Reports healthy only when the bytes were initialized (set at least once).
type eskipBytesStatus struct {
	b *eskipBytes
}

const msgRoutesNotInitialized = "routes were not initialized yet"

func (s *eskipBytesStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, initialized := s.b.bytes(); initialized {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, msgRoutesNotInitialized, http.StatusServiceUnavailable)
	}
}
