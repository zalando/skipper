package routesrv

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	ot "github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

// eskipBytes keeps eskip-formatted routes as a byte slice and
// provides synchronized r/w access to them. Additionally it can
// serve as an HTTP handler exposing its content.
type eskipBytes struct {
	data         []byte
	etag         string
	lastModified time.Time
	initialized  bool
	count        int
	mu           sync.RWMutex

	tracer ot.Tracer
}

// bytes returns a slice to stored bytes, which are safe for reading,
// and if there were already initialized.
func (e *eskipBytes) bytes() ([]byte, string, time.Time, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.data, e.etag, e.lastModified, e.initialized
}

// formatAndSet takes a slice of routes and stores them eskip-formatted
// in a synchronized way. It returns a number of stored bytes and a boolean,
// being true, when the stored bytes were set for the first time.
func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (int, bool) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}, routes...)

	e.mu.Lock()
	defer e.mu.Unlock()
	if updated := buf.Bytes(); !bytes.Equal(e.data, updated) {
		e.lastModified = time.Now()
		e.data = updated
		h := sha256.New()
		h.Write(e.data)
		e.etag = fmt.Sprintf(`"%x"`, h.Sum(nil))
	}
	oldInitialized := e.initialized
	e.initialized = true
	e.count = len(routes)

	return len(e.data), !oldInitialized
}

func (e *eskipBytes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := tracing.CreateSpan("serve_routes", r.Context(), e.tracer)
	defer span.Finish()

	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	data, etag, lastModified, initialized := e.bytes()
	if initialized {
		w.Header().Add("Etag", etag)
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.Header().Add(routing.RoutesCountName, strconv.Itoa(e.count))

		http.ServeContent(w, r, "", lastModified, bytes.NewReader(data))
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
	if _, _, _, initialized := s.b.bytes(); initialized {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, msgRoutesNotInitialized, http.StatusServiceUnavailable)
	}
}
