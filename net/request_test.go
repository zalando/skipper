package net

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
			name:         "no match multiple",
			request:      testRequest,
			values:       []string{"none", "neither"},
			expectStatus: 200,
		},
		{
			name:         "match uri path",
			request:      testRequest,
			values:       []string{"one"},
			expectStatus: 400,
		},
		{
			name:         "match uri path multiple",
			request:      testRequest,
			values:       []string{"none", "one"},
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

var benchmarkMatchesRequestSink = false

func BenchmarkMatchesRequest(b *testing.B) {
	const target = "Target"
	fake := func(len int) string {
		return strings.Repeat(target[:2], len/2) // partially matches target
	}
	h := RequestMatchHandler{Match: []string{target}, Handler: nil}
	r := &http.Request{
		RequestURI: fake(170),
		Header: http.Header{
			"Pragma":          []string{"no-cache"},
			"Cache-Control":   []string{"no-cache"},
			"Accept":          []string{"*/*"},
			"Accept-Language": []string{fake(30)},
			"User-Agent":      []string{fake(100)},
			"Cookie":          []string{fake(2600)},
			"Referer":         []string{fake(170)},
			"X-Xsrf-Token":    []string{fake(150)},
		},
	}
	if h.matchesRequest(r) {
		b.Fatal("expected no match")
	}
	b.ResetTimer()
	m := false
	for i := 0; i < b.N; i++ {
		m = h.matchesRequest(r)
	}
	benchmarkMatchesRequestSink = m
}
