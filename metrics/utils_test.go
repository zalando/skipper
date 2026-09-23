package metrics

import (
	"testing"
)

func TestMeasuredMethod(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"GET", "GET"},
		{"HEAD", "HEAD"},
		{"POST", "POST"},
		{"PUT", "PUT"},
		{"PATCH", "PATCH"},
		{"DELETE", "DELETE"},
		{"TRACE", "TRACE"},
		{"CONNECT", "CONNECT"},
		{"OPTIONS", "OPTIONS"},
		{"QUERY", "QUERY"},
		{"UNKNOWN", "_unknownmethod_"},
		{"custom", "_unknownmethod_"},
		{"", "_unknownmethod_"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if actual := measuredMethod(tc.input); actual != tc.expected {
				t.Fatalf("expected %q for method %q, got %q", tc.expected, tc.input, actual)
			}
		})
	}
}
