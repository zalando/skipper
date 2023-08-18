package admission

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
			message:   "error in route group n1/rg1: route group without spec",
		},
		{
			name:      "valid eskip filters",
			inputFile: "rg-with-valid-eskip-filters.json",
		},
		{
			name:      "invalid eskip filters",
			inputFile: "rg-with-invalid-eskip-filters.json",
			message:   "parse failed after token status, last route id: , position 11: syntax error",
		},
		{
			name:      "valid eskip predicates",
			inputFile: "rg-with-valid-eskip-predicates.json",
		},
		{
			name:      "invalid eskip predicates",
			inputFile: "rg-with-invalid-eskip-predicates.json",
			message:   "parse failed after token Method, last route id: Method, position 6: syntax error",
		},
		{
			name:      "invalid eskip filters and predicates",
			inputFile: "rg-with-invalid-eskip-filters-and-predicates.json",
			message:   "parse failed after token status, last route id: , position 11: syntax error\\nparse failed after token Method, last route id: Method, position 6: syntax error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedResponse := responseAllowedFmt
			if len(tc.message) > 0 {
				expectedResponse = fmt.Sprintf(responseNotAllowedFmt, tc.message)
			}

			input, err := os.ReadFile("testdata/rg/" + tc.inputFile)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "http://example.com/foo", bytes.NewBuffer(input))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			rgAdm := &RouteGroupAdmitter{}

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

func TestIngressAdmitter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inputFile string
		message   string
	}{
		{
			name:      "allowed without annotations",
			inputFile: "valid-ingress-without-annotations.json",
		},
		{
			name:      "allowed with valid annotations",
			inputFile: "valid-ingress-with-annotations.json",
		},
		{
			name:      "invalid eskip filters",
			inputFile: "invalid-filters.json",
			message:   `invalid \"zalando.org/skipper-filter\" annotation: parse failed after token this, last route id: , position 9: syntax error`,
		},
		{
			name:      "invalid eskip predicates",
			inputFile: "invalid-predicates.json",
			message:   `invalid \"zalando.org/skipper-predicate\" annotation: parse failed after token ), last route id: , position 15: syntax error`,
		},
		{
			name:      "invalid eskip routes",
			inputFile: "invalid-routes.json",
			message:   `invalid \"zalando.org/skipper-routes\" annotation: invalid predicate count arg`,
		},
		{
			name:      "invalid eskip filters and predicates",
			inputFile: "invalid-filters-and-predicates.json",
			message:   `invalid \"zalando.org/skipper-filter\" annotation: parse failed after token this, last route id: , position 9: syntax error\ninvalid \"zalando.org/skipper-predicate\" annotation: parse failed after token ), last route id: , position 15: syntax error`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectedResponse := responseAllowedFmt
			if len(tc.message) > 0 {
				expectedResponse = fmt.Sprintf(responseNotAllowedFmt, tc.message)
			}

			input, err := os.ReadFile("testdata/ingress/" + tc.inputFile)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "http://example.com/foo", bytes.NewBuffer(input))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			ingressAdm := &IngressAdmitter{}

			h := Handler(ingressAdm)
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

func TestMalformedRequests(t *testing.T) {
	routeGroupHandler := Handler(&RouteGroupAdmitter{})
	ingressHandler := Handler(&IngressAdmitter{})

	for _, tc := range []struct {
		name           string
		method         string
		contentType    string
		input          string
		expectedStatus int
	}{
		{
			name:           "unsupported method",
			method:         "GET",
			input:          `{"foo": "bar"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unsupported content type",
			contentType:    "text/plain",
			input:          "hello world",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed json",
			input:          "not a json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing review request",
			input:          `{"foo": "bar"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "malformed object",
			input: `
			  {
				"request": {
				  "uid": "req-uid",
				  "name": "req1",
				  "namespace": "ns1",
				  "object": "not an object"
				}
			  }
			`,
			expectedStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makeRequest := func() *http.Request {
				method := "POST"
				if tc.method != "" {
					method = tc.method
				}

				req := httptest.NewRequest(method, "http://example.com/foo", strings.NewReader(tc.input))
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				} else {
					req.Header.Set("Content-Type", "application/json")
				}
				return req
			}

			t.Run("route group", func(t *testing.T) {
				w := httptest.NewRecorder()
				routeGroupHandler(w, makeRequest())
				assert.Equal(t, tc.expectedStatus, w.Code)
			})

			t.Run("ingress", func(t *testing.T) {
				w := httptest.NewRecorder()
				ingressHandler(w, makeRequest())
				assert.Equal(t, tc.expectedStatus, w.Code)
			})
		})
	}
}
