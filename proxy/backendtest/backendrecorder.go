package backendtest

import (
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
)

type RecordedRequest struct {
	URL  *url.URL
	Body string
}

type Done chan struct{}

type backendRecorderHandler struct {
	server           *httptest.Server
	requests         []RecordedRequest
	mutex            sync.RWMutex
	expectedRequests int
	pendingRequests  int
	Done             Done
}

func (rec *backendRecorderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("backendrecorder: error while reading request body")
	}
	// return request body in the response
	_, err = w.Write(body)
	if err != nil {
		log.Error("backendrecorder: error while writing the response body")
	}
	rec.mutex.Lock()
	rec.pendingRequests--
	rec.requests = append(rec.requests, RecordedRequest{
		URL:  r.URL,
		Body: string(body),
	})
	if rec.pendingRequests == 0 {
		close(rec.Done)
	}
	rec.mutex.Unlock()
}

func (rec *backendRecorderHandler) GetRequests() []RecordedRequest {
	rec.mutex.RLock()
	requests := rec.requests
	rec.mutex.RUnlock()
	return requests
}

func (rec *backendRecorderHandler) GetPendingRequests() int {
	rec.mutex.RLock()
	expected := rec.expectedRequests - rec.pendingRequests
	rec.mutex.RUnlock()
	return expected
}

func (rec *backendRecorderHandler) GetServedRequests() int {
	rec.mutex.RLock()
	served := rec.expectedRequests - rec.pendingRequests
	rec.mutex.RUnlock()
	return served
}

func (rec *backendRecorderHandler) GetExpectedRequests() int {
	rec.mutex.RLock()
	expected := rec.expectedRequests
	rec.mutex.RUnlock()
	return expected
}

func (rec *backendRecorderHandler) GetURL() string {
	return rec.server.URL
}

func NewBackendRecorder(expectedRequests int) *backendRecorderHandler {
	handler := &backendRecorderHandler{
		pendingRequests:  expectedRequests,
		expectedRequests: expectedRequests,
		Done:             make(chan struct{}),
	}
	if expectedRequests == 0 {
		close(handler.Done)
	}
	server := httptest.NewServer(handler)
	handler.server = server
	return handler
}
