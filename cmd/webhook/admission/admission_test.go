package admission

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		name   string
		input  string
		result string
	}{
		{
			name: "allowed",
			input: `{
				"request": {
					"uid": "req-uid",
					"name": "req1",
					"namespace": "n1",
					"object": {
						"metadata": {
							"name": "rg1",
							"namespace": "n1"
						},
						"spec": {
							"backends": [
								{
									"name": "backend",
									"type": "shunt"
								}
							],
							"defaultBackends": [
								{
									"backendName": "backend"
								}
							]
						}
					}
				}
			}`,
			result: `{
				"kind": "AdmissionReview",
				"apiVersion": "admission.k8s.io/v1",
				"response": {
					"uid": "req-uid",
					"allowed": true
				}
			}`,
		},
		{
			name: "not allowed",
			input: `{
				"request": {
					"uid": "req-uid",
					"name": "req1",
					"namespace": "n1",
					"object": {
						"metadata": {
							"name": "rg1",
							"namespace": "n1"
						}
					}
				}
			}`,
			result: `{
				"kind": "AdmissionReview",
				"apiVersion": "admission.k8s.io/v1",
				"response": {
					"uid": "req-uid",
					"allowed": false,
					"status": {
						"message":
						"could not validate RouteGroup, error in route group n1/rg1: route group without spec"
					}
				}
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/foo", bytes.NewBuffer([]byte(tc.input)))
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

			assert.JSONEq(t, tc.result, string(rb))
		})
	}
}
