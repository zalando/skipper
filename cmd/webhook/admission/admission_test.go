package admission

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	zv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	admissionsv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testAdmitter struct {
	// validate validates & plugs parameters for Admit
	validate func(response *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error)
}

func (t testAdmitter) Name() string {
	return "testRouteGroup"
}

func (t testAdmitter) Admit(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error) {
	return t.validate(req)
}

func (t testAdmitter) AdmitAll(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error) {
	return &admissionsv1.AdmissionResponse{
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
	expectedRg := zv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "r1",
			Namespace: "n1",
		},
	}

	rb, err := json.Marshal(expectedRg)
	assert.NoError(t, err)

	review := &admissionsv1.AdmissionReview{
		Request: &admissionsv1.AdmissionRequest{
			Name:      "r1",
			Namespace: "n1",
			// TODO: why doesnt runtime.RawExtension{Object: rg} work here?
			Object: runtime.RawExtension{Raw: rb},
		},
	}

	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)
	err = enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()

	tadm.validate = func(req *admissionsv1.AdmissionRequest) (*admissionsv1.AdmissionResponse, error) {
		// TODO: add a differ here so the message is more readable
		assert.Equal(t, review.Request, req, "AdmissionReview.Request is not as expected")

		// decode a RouteGroup from req.Object.Raw and validate it
		var rg definitions.RouteGroupItem
		err := json.Unmarshal(req.Object.Raw, &rg)
		assert.NoError(t, err)

		assert.Equal(t, expectedRg.Name, rg.Metadata.Name)
		assert.Equal(t, expectedRg.Namespace, rg.Metadata.Namespace)

		return &admissionsv1.AdmissionResponse{
			Allowed: true,
		}, nil
	}

	h := Handler(tadm)
	h(w, req)

}

func TestResponseEncoding(t *testing.T) {
	review := &admissionsv1.AdmissionReview{
		Request: &admissionsv1.AdmissionRequest{
			Name:      "r1",
			Namespace: "n1",
		},
	}
	expectedResp := &admissionsv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: "failed to validate",
		},
	}
	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)
	err := enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	tadm := NewTestAdmitter()
	tadm.validate = func(ar *admissionsv1.AdmissionRequest) (
		*admissionsv1.AdmissionResponse,
		error,
	) {
		return expectedResp, nil
	}

	h := Handler(tadm)
	h(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	reviewResp := admissionsv1.AdmissionReview{}
	rb, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err, "could not read response")

	err = json.Unmarshal(rb, &reviewResp)
	if assert.NoError(t, err) {
		assert.Equal(t, expectedResp, reviewResp.Response)
	}
}

// Test RouteGroupAdmitter.Admit
func TestAdmitRouteGroups(t *testing.T) {
	rg := zv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "r1",
			Namespace: "n1",
		},
	}

	rgB, err := json.Marshal(rg)
	assert.NoError(t, err)

	review := &admissionsv1.AdmissionReview{
		Request: &admissionsv1.AdmissionRequest{
			UID:       "uid",
			Name:      "r1",
			Namespace: "n1",
			Object:    runtime.RawExtension{Raw: rgB},
		},
	}

	bbuffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(bbuffer)
	err = enc.Encode(review)
	assert.NoError(t, err, "could not encode admission review")

	req := httptest.NewRequest("POST", "http://example.com/foo", bbuffer)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	rgAdm := RouteGroupAdmitter{}

	h := Handler(rgAdm)
	h(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respReview := &admissionsv1.AdmissionReview{}
	rb, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err, "could not read response")
	err = json.Unmarshal(rb, &respReview)
	assert.NoError(t, err)

	// Request Response UID should match
	assert.Equal(t, review.Request.UID, respReview.Response.UID)

}

func TestExtractName(t *testing.T) {
	rg := zv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "r2",
			Namespace: "n1",
		},
	}

	rb, err := json.Marshal(rg)
	assert.NoError(t, err)

	request := &admissionsv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: rb},
	}
	name := extractName(request)
	assert.Equal(t, rg.Name, name)
}
