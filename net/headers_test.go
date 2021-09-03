package net

import (
	"net/http"
	"reflect"
	"testing"
)

func TestForwardedHeaders(t *testing.T) {
	for _, ti := range []struct {
		name       string
		remoteAddr string
		header     http.Header
		forwarded  ForwardedHeaders
		expected   http.Header
	}{
		{
			name:       "no change when disabled",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
			forwarded: ForwardedHeaders{},
			expected: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
		},
		{
			name:       "no remote",
			remoteAddr: "",
			header: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
			forwarded: ForwardedHeaders{For: true},
			expected: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
		},
		{
			name:       "set xff",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{For: true},
			expected: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4"},
			},
		},
		{
			name:       "append xff",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
			forwarded: ForwardedHeaders{For: true},
			expected: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1, 1.2.3.4"},
			},
		},
		{
			name:       "prepend xff",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
			forwarded: ForwardedHeaders{PrependFor: true},
			expected: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4, 4.3.2.1"},
			},
		},
		{
			name:       "prepend xff overrides",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-For": []string{"4.3.2.1"},
			},
			forwarded: ForwardedHeaders{For: true, PrependFor: true},
			expected: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4, 4.3.2.1"},
			},
		},
		{
			name:       "set xff, host, proto",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{For: true, Host: true, Proto: "https"},
			expected: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4"},
				"X-Forwarded-Host":  []string{"example.com"},
				"X-Forwarded-Proto": []string{"https"},
			},
		},
		{
			name:       "set xff, overwrite host, proto",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-Host":  []string{"whatever"},
				"X-Forwarded-Proto": []string{"whatever"},
			},
			forwarded: ForwardedHeaders{For: true, Host: true, Proto: "https"},
			expected: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4"},
				"X-Forwarded-Host":  []string{"example.com"},
				"X-Forwarded-Proto": []string{"https"},
			},
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			r := &http.Request{Host: "example.com", RemoteAddr: ti.remoteAddr, Header: ti.header}

			ti.forwarded.Set(r)

			if !reflect.DeepEqual(ti.expected, r.Header) {
				t.Errorf("header mismatch, expected: %v, got: %v", ti.expected, r.Header)
			}
		})
	}
}
