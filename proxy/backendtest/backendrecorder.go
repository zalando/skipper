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
	done     <-chan time.Time
	mu       sync.RWMutex
	requests []RecordedRequest
}

func NewBackendRecorder(closeAfter time.Duration) *BackendRecorderHandler {
	handler := &BackendRecorderHandler{
		done: time.After(closeAfter),
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
	rec.mu.Lock()
	rec.requests = append(rec.requests, RecordedRequest{
		URL:  r.URL,
		Body: string(body),
	})
	rec.mu.Unlock()
}

func (rec *BackendRecorderHandler) GetRequests() []RecordedRequest {
	rec.mu.RLock()
	requests := rec.requests
	rec.mu.RUnlock()
	return requests
}

func (rec *BackendRecorderHandler) GetURL() string {
	return rec.server.URL
}

func (rec *BackendRecorderHandler) Done() {
	<-rec.done
	rec.server.Close()
}
