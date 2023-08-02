package admission

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	responseAllowedFmt = `{
		"kind": "AdmissionReview",
		"apiVersion": "admission.k8s.io/v1",
		"response": {
			"uid": "req-uid",
			"allowed": true
		}
	}`

	responseNotAllowedFmt = `{
		"kind": "AdmissionReview",
		"apiVersion": "admission.k8s.io/v1",
		"response": {
			"uid": "req-uid",
			"allowed": false,
			"status": {
				"message": "%s"
			}
		}
	}`
)

type testAdmitter struct {
	// validate validates & plugs parameters for Admit
	validate func(response *admissionRequest) (*admissionResponse, error)
}

func (t testAdmitter) name() string {
	return "testRouteGroup"
}

func (t testAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	return t.validate(req)
}

func (t testAdmitter) admitAll(req *admissionRequest) (*admissionResponse, error) {
	return &admissionResponse{
		Allowed: true,
	}, nil
}

func NewTestAdmitter() *testAdmitter {
	tadm := &testAdmitter{}
	tadm.validate = tadm.admitAll
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

func TestRouteGroupAdmitter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inputFile string
		message   string
	}{
		{
			name:      "allowed",
			inputFile: "valid-rg.json",
		},
		{
			name:      "not allowed",
			inputFile: "invalid-rg.json",
			message:   "could not validate RouteGroup, error in route group n1/rg1: route group without spec",
		},
		{
			name:      "valid eskip filters",
			inputFile: "rg-with-valid-eskip-filters.json",
		},
		{
			name:      "invalid eskip filters",
			inputFile: "rg-with-invalid-eskip-filters.json",
			message:   "could not validate RouteGroup, parse failed after token status, last route id: , position 11: syntax error",
		},
		{
			name:      "valid eskip predicates",
			inputFile: "rg-with-valid-eskip-predicates.json",
		},
		{
			name:      "invalid eskip predicates",
			inputFile: "rg-with-invalid-eskip-predicates.json",
			message:   "could not validate RouteGroup, parse failed after token Method, last route id: Method, position 6: syntax error",
		},
		{
			name:      "invalid eskip filters and predicates",
			inputFile: "rg-with-invalid-eskip-filters-and-predicates.json",
			message:   "could not validate RouteGroup, parse failed after token status, last route id: , position 11: syntax error\\nparse failed after token Method, last route id: Method, position 6: syntax error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedResponse := responseAllowedFmt
			if len(tc.message) > 0 {
				expectedResponse = fmt.Sprintf(responseNotAllowedFmt, tc.message)
			}

			input, err := os.ReadFile("testdata/" + tc.inputFile)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "http://example.com/foo", bytes.NewBuffer(input))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			rgAdm := RouteGroupAdmitter{}

			h := Handler(rgAdm)
			h(w, req)
			resp := w.Result()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			rb, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			resp.Body.Close()

			assert.JSONEq(t, expectedResponse, string(rb))
		})
	}
}
