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
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/tracing"
)

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *responseWriterInterceptor) Write(p []byte) (int, error) {
	w.bytesWritten += len(p)
	return w.ResponseWriter.Write(p)
}

// Unwrap will be used by ResponseController, so if they will use that
// to get the ResponseWrite for some reason they can do it.
func (w *responseWriterInterceptor) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

var (
	_ http.ResponseWriter = &responseWriterInterceptor{}
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

	tracer  ot.Tracer
	metrics metrics.Metrics
	now     func() time.Time
}

// formatAndSet takes a slice of routes and stores them eskip-formatted
// in a synchronized way. It returns the length of the stored data, and
// flags signaling whether the data was initialized and updated.
func (e *eskipBytes) formatAndSet(routes []*eskip.Route) (_ int, _ string, initialized bool, updated bool) {
	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}, routes...)
	data := buf.Bytes()

	e.mu.Lock()
	defer e.mu.Unlock()

	updated = !bytes.Equal(e.data, data)
	if updated {
		e.lastModified = e.now()
		e.data = data
		e.etag = fmt.Sprintf(`"%x"`, sha256.Sum256(e.data))
		e.count = len(routes)
	}
	initialized = !e.initialized
	e.initialized = true

	return len(e.data), e.etag, initialized, updated
}

func (e *eskipBytes) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	span := tracing.CreateSpan("serve_routes", r.Context(), e.tracer)
	defer span.Finish()
	start := time.Now()
	defer e.metrics.MeasureBackend("routersv", start)

	w := &responseWriterInterceptor{
		ResponseWriter: rw,
		statusCode:     http.StatusOK,
	}

	defer func() {
		span.SetTag("status", w.statusCode)
		span.SetTag("bytes", w.bytesWritten)

		e.metrics.IncCounter(strconv.Itoa(w.statusCode))
	}()

	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	e.mu.RLock()
	count := e.count
	data := e.data
	etag := e.etag
	lastModified := e.lastModified
	initialized := e.initialized
	e.mu.RUnlock()

	if initialized {
		w.Header().Add("Etag", etag)
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.Header().Add(routing.RoutesCountName, strconv.Itoa(count))
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
	s.b.mu.RLock()
	initialized := s.b.initialized
	s.b.mu.RUnlock()
	if initialized {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, msgRoutesNotInitialized, http.StatusServiceUnavailable)
	}
}
