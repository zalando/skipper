package net

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryHandler(t *testing.T) {
	for _, ti := range []struct {
		name     string
		routes   string
		query    string
		expected int
	}{
		{
			name:     "request without query",
			routes:   `r: * -> inlineContent("OK") -> <shunt>;`,
			query:    "",
			expected: http.StatusOK,
		},
		{
			name:     "request with query",
			routes:   `r1: Query("foo") -> status(400) -> inlineContent("FAIL") -> <shunt> ;r2: * -> inlineContent("OK") -> <shunt>;`,
			query:    "foo=bar",
			expected: http.StatusOK,
		},
		{
			name:     "request with bad query",
			routes:   `r1: Query("foo") -> status(400) -> inlineContent("FAIL") -> <shunt> ;r2: * -> inlineContent("OK") -> <shunt>;`,
			query:    "foo=bar;",
			expected: http.StatusBadRequest,
		},
	} {
		t.Run(ti.name, func(t *testing.T) {

			req, err := http.NewRequest("GET", "/path", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.URL.RawQuery = ti.query
			req.RemoteAddr = "1.2.3.4:5678"
			noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			recorder := httptest.NewRecorder()
			h := &ValidateQueryHandler{
				Handler: noop,
			}
			h.ServeHTTP(recorder, req)

			if recorder.Code != ti.expected {
				t.Fatalf("Failed to get expected code %d, got %d", ti.expected, recorder.Code)
			}
		})
	}
}

func TestQueryLogHandler(t *testing.T) {
	for _, ti := range []struct {
		name     string
		routes   string
		query    string
		expected int
	}{
		{
			name:     "request without query",
			routes:   `r: * -> inlineContent("OK") -> <shunt>;`,
			query:    "",
			expected: http.StatusOK,
		},
		{
			name:     "request with query",
			routes:   `r1: Query("foo") -> status(400) -> inlineContent("FAIL") -> <shunt> ;r2: * -> inlineContent("OK") -> <shunt>;`,
			query:    "foo=bar",
			expected: http.StatusOK,
		},
		{
			name:     "request with bad query does not block",
			routes:   `r1: Query("foo") -> status(400) -> inlineContent("FAIL") -> <shunt> ;r2: * -> inlineContent("OK") -> <shunt>;`,
			query:    "foo=bar;",
			expected: http.StatusOK,
		},
	} {
		t.Run(ti.name, func(t *testing.T) {

			req, err := http.NewRequest("GET", "/path", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.URL.RawQuery = ti.query
			req.RemoteAddr = "1.2.3.4:5678"
			noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			recorder := httptest.NewRecorder()
			h := &ValidateQueryLogHandler{
				Handler: noop,
			}
			h.ServeHTTP(recorder, req)

			if recorder.Code != ti.expected {
				t.Fatalf("Failed to get expected code %d, got %d", ti.expected, recorder.Code)
			}
		})
	}
}
