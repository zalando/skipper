package routesrv

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	mu               sync.RWMutex
	hash             string
	zoneHash         map[string]string
	lastModified     time.Time
	zoneLastModified map[string]time.Time
	initialized      bool
	count            int
	zoneCount        map[string]int

	zw                 *gzip.Writer
	zdata              []byte
	zoneDataCompressed map[string][]byte

	tracer  ot.Tracer
	metrics metrics.Metrics
	now     func() time.Time
}

// formatAndSet takes a slice of routes and stores them eskip-formatted
// in a synchronized way. It returns the length of the stored data, and
// flags signaling whether the data was initialized and updated.
// Each zone's routes are formatted and hashed exactly once per call; only
// the zones whose hash actually changed are recompressed and get a fresh
// Last-Modified. Unchanged zones keep their existing Etag/Last-Modified/
// compressed bytes untouched, and zones that disappeared entirely are
// removed so requests for them fall back to the global routing table.
func (e *eskipBytes) formatAndSet(routes []*eskip.Route, zoneAwareRoutes map[string][]*eskip.Route) (_ int, _ string, initialized bool, updated bool) {

	buf := &bytes.Buffer{}
	eskip.Fprint(buf, eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}, routes...)
	data := buf.Bytes()

	e.mu.Lock()
	defer e.mu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	updated = hash != e.hash

	if updated {
		now := e.now()
		if e.zoneHash == nil {
			e.zoneHash = make(map[string]string)
			e.zoneDataCompressed = make(map[string][]byte)
			e.zoneCount = make(map[string]int)
			e.zoneLastModified = make(map[string]time.Time)
		}

		// Format and hash every zone exactly once, so unchanged zones
		// never need to be recompressed below.
		newZoneHash := make(map[string]string, len(zoneAwareRoutes))
		for zone, zoneRoutes := range zoneAwareRoutes {
			zoneBuf := &bytes.Buffer{}
			eskip.Fprint(zoneBuf, eskip.PrettyPrintInfo{Pretty: false, IndentStr: ""}, zoneRoutes...)
			zoneData := zoneBuf.Bytes()
			zoneHash := fmt.Sprintf("%x", sha256.Sum256(zoneData))
			newZoneHash[zone] = zoneHash

			if e.zoneHash[zone] == zoneHash {
				continue // this zone's content is unchanged; leave it alone
			}
			e.zoneHash[zone] = zoneHash
			e.zoneDataCompressed[zone] = e.compressLocked(zoneData)
			e.zoneCount[zone] = len(zoneRoutes)
			e.zoneLastModified[zone] = now
		}
		for zone := range e.zoneHash {
			if _, ok := newZoneHash[zone]; !ok {
				// zone no longer present at all - remove it so ServeHTTP
				// falls back to the global routing table for it instead
				// of serving stale data.
				delete(e.zoneHash, zone)
				delete(e.zoneDataCompressed, zone)
				delete(e.zoneCount, zone)
				delete(e.zoneLastModified, zone)
			}
		}
		e.lastModified = now
		e.zdata = e.compressLocked(data)
		e.hash = hash
		e.count = len(routes)
	}
	initialized = !e.initialized
	e.initialized = true

	return len(data), e.hash, initialized, updated
}

// compressLocked compresses the data with gzip and returns
// the compressed data or nil if compression fails.
// e.mu must be held.
func (e *eskipBytes) compressLocked(data []byte) []byte {
	var buf bytes.Buffer
	if e.zw == nil {
		e.zw = gzip.NewWriter(&buf)
	} else {
		e.zw.Reset(&buf)
	}
	if _, err := e.zw.Write(data); err != nil {
		return nil
	}
	if err := e.zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

func decompress(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
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
	zdata := e.zdata
	hash := e.hash
	lastModified := e.lastModified
	initialized := e.initialized

	// if a zone is specified and there are at least three endpoints for that are zone, serve the zone aware routes
	// else serve all routes
	// we check for three endpoints because three is good choice for availability; it guarantees that at least one endpoint will be healthy even if the others are on update or have some issues
	zone := r.PathValue("zone")
	if zone != "" {
		if zd, ok := e.zoneDataCompressed[zone]; ok {
			zdata = zd
			count = e.zoneCount[zone]
			hash = e.zoneHash[zone]
			lastModified = e.zoneLastModified[zone]
		}
	}

	e.mu.RUnlock()

	if initialized {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set(routing.RoutesCountName, strconv.Itoa(count))

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(zdata) > 0 {
			w.Header().Set("Etag", `"`+hash+`+gzip"`)
			w.Header().Set("Content-Encoding", "gzip")
			http.ServeContent(w, r, "", lastModified, bytes.NewReader(zdata))
		} else {
			data, err := decompress(zdata)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Etag", `"`+hash+`"`)
			http.ServeContent(w, r, "", lastModified, bytes.NewReader(data))
		}
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
