package net

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestMatchHandler(t *testing.T) {
	testRequest := &http.Request{
		RequestURI: "/one?two=three&four=five",
		Header: http.Header{
			"six":  []string{"seven", "eight"},
			"nine": []string{"ten", "eleven"},
		},
	}

	for _, ti := range []struct {
		name         string
		request      *http.Request
		values       []string
		expectStatus int
	}{
		{
			name:         "no match",
			request:      testRequest,
			values:       []string{"none"},
			expectStatus: 200,
		},
		{
			name:         "match uri path",
			request:      testRequest,
			values:       []string{"one"},
			expectStatus: 400,
		},
		{
			name:         "match uri path escaped",
			request:      &http.Request{RequestURI: "/%78%78%78?aaa=aaa"},
			values:       []string{"xxx"},
			expectStatus: 400,
		},
		{
			name:         "match uri query param name escaped",
			request:      &http.Request{RequestURI: "/aaa?%78%78%78=aaa"},
			values:       []string{"xxx"},
			expectStatus: 400,
		},
		{
			name:         "match uri query param value escaped",
			request:      &http.Request{RequestURI: "/aaa?aaa=%78%78%78"},
			values:       []string{"xxx"},
			expectStatus: 400,
		},
		{
			name:         "match uri query param name",
			request:      testRequest,
			values:       []string{"two"},
			expectStatus: 400,
		},
		{
			name:         "match uri query param value",
			request:      testRequest,
			values:       []string{"three"},
			expectStatus: 400,
		},
		{
			name:         "match header name",
			request:      testRequest,
			values:       []string{"six"},
			expectStatus: 400,
		},
		{
			name:         "match header value",
			request:      testRequest,
			values:       []string{"seven"},
			expectStatus: 400,
		},
		{
			name:         "match header second value",
			request:      testRequest,
			values:       []string{"eight"},
			expectStatus: 400,
		},
		{
			name:         "match second header name",
			request:      testRequest,
			values:       []string{"nine"},
			expectStatus: 400,
		},
		{
			name:         "match second header value",
			request:      testRequest,
			values:       []string{"ten"},
			expectStatus: 400,
		},
		{
			name:         "match second header second value",
			request:      testRequest,
			values:       []string{"eleven"},
			expectStatus: 400,
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			recorder := httptest.NewRecorder()

			h := RequestMatchHandler{Match: ti.values, Handler: backend}

			h.ServeHTTP(recorder, ti.request)

			resp := recorder.Result()

			if resp.StatusCode != ti.expectStatus {
				t.Errorf("wrong status, expected: %d, got: %d", ti.expectStatus, resp.StatusCode)
			}
		})
	}
}
