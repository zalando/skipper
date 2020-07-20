package admission

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
)

func TestNonPostRequestsBad(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	Handler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnsupportedContentTyp(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	Handler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandler(t *testing.T) {
	reviewreq := &admissionv1.AdmissionRequest{}
	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)

	err := enc.Encode(reviewreq)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	Handler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	admresp := &admissionv1.AdmissionResponse{}
	rb, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err, "could not read response")

	err = json.Unmarshal(rb, &admresp)
	assert.NoError(t, err, "could not parse admission response")
}
