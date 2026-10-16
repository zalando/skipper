package awssigner

import (
	"sync"
	"testing"
)

func TestEscapePath(t *testing.T) {
	testCases := []struct {
		name      string
		path      string
		encodeSep bool
		expected  string
	}{
		{
			name:      "unreserved characters untouched",
			path:      "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~",
			encodeSep: false,
			expected:  "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~",
		},
		{
			name:      "path with slash encodeSep false",
			path:      "/foo/bar/baz",
			encodeSep: false,
			expected:  "/foo/bar/baz",
		},
		{
			name:      "path with slash encodeSep true",
			path:      "/foo/bar/baz",
			encodeSep: true,
			expected:  "%2Ffoo%2Fbar%2Fbaz",
		},
		{
			name:      "spaces and special characters escaped",
			path:      "/hello world@123!",
			encodeSep: false,
			expected:  "/hello%20world%40123%21",
		},
		{
			name:      "empty path",
			path:      "",
			encodeSep: false,
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapePath(tc.path, tc.encodeSep)
			if got != tc.expected {
				t.Errorf("EscapePath(%q, %v) = %q; want %q", tc.path, tc.encodeSep, got, tc.expected)
			}
		})
	}
}

func TestEscapePath_ConcurrentRace(t *testing.T) {
	const workers = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = EscapePath("/test/path/to/resource?param=value&foo=bar", false)
				_ = EscapePath("/test/path/to/resource?param=value&foo=bar", true)
			}
		}()
	}

	wg.Wait()
}
