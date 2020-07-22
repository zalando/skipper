package admission

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
)

type testAdmitter struct {
	// validate validates & plugs parameters for Admit
	validate func(response *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error)
}

func (t testAdmitter) Admit(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
	return t.validate(req)
}

func (t testAdmitter) AdmitAll(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
	return admissionv1.AdmissionResponse{
		Allowed: true,
	}, nil
}

func NewTestAdmitter() *testAdmitter {
	tadm := &testAdmitter{}
	tadm.validate = tadm.AdmitAll
	return tadm
}

func TestNonPostRequestsBad(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()
	h := Handler(tadm)
	h(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnsupportedContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()
	h := Handler(tadm)
	h(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRequestDecoding(t *testing.T) {
	review := &admissionv1.AdmissionReview{}
	review.Request = &admissionv1.AdmissionRequest{
		Name:      "r1",
		Namespace: "n1",
	}

	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)

	err := enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()

	tadm.validate = func(req *admissionv1.AdmissionRequest) (admissionv1.AdmissionResponse, error) {
		// TODO: add a differ here so the message is more readable
		assert.Equal(t, review.Request, req, "AdmissionReview.Request is not as expected")

		return admissionv1.AdmissionResponse{
			Allowed: true,
		}, nil
	}

	h := Handler(tadm)
	h(w, req)

}

func TestResponseEncoding(t *testing.T) {
	// resp := w.Result()
	// assert.Equal(t, http.StatusOK, resp.StatusCode)
	//
	// admresp := &admissionv1.AdmissionResponse{}
	// rb, err := ioutil.ReadAll(resp.Body)
	// assert.NoError(t, err, "could not read response")
	// // TODO: instead of just looking at the response, write a test admitter
	// //  that verifies the AdmissionReview{} Received and sends back a test
	// //  response
	// err = json.Unmarshal(rb, &admresp)
	// assert.NoError(t, err, "could not parse admission response")
}
