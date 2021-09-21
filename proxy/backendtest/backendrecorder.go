package backendtest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type RecordedRequest struct {
	URL  *url.URL
	Body string
}

type BackendRecorderHandler struct {
	server   *httptest.Server
	requests []RecordedRequest
	mutex    sync.RWMutex
	Done     <-chan time.Time
}

func NewBackendRecorder(closeAfter time.Duration) *BackendRecorderHandler {
	handler := &BackendRecorderHandler{
		Done: time.After(closeAfter),
	}
	server := httptest.NewServer(handler)
	handler.server = server
	return handler
}

func (rec *BackendRecorderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("backendrecorder: error while reading request body: %v", err)
	}
	// return request body in the response
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("backendrecorder: error writing reading request body: %v", err)
	}
	rec.mutex.Lock()
	rec.requests = append(rec.requests, RecordedRequest{
		URL:  r.URL,
		Body: string(body),
	})
	rec.mutex.Unlock()
}

func (rec *BackendRecorderHandler) GetRequests() []RecordedRequest {
	rec.mutex.RLock()
	requests := rec.requests
	rec.mutex.RUnlock()
	return requests
}

func (rec *BackendRecorderHandler) GetURL() string {
	return rec.server.URL
}

func (rec *BackendRecorderHandler) Close() {
	rec.server.Close()
}
