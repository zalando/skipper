package net

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestForwardedHeaders(t *testing.T) {
	for _, ti := range []struct {
		name       string
		remoteAddr string
		method     string
		requestURI string
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
			name:       "set xff, host, port, proto, method, uri",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{For: true, Host: true, Method: true, Uri: true, Port: "443", Proto: "https"},
			method:     "POST",
			requestURI: "/foo?bar=baz",
			expected: http.Header{
				"X-Forwarded-For":    []string{"1.2.3.4"},
				"X-Forwarded-Host":   []string{"example.com"},
				"X-Forwarded-Method": []string{"POST"},
				"X-Forwarded-Uri":    []string{"/foo?bar=baz"},
				"X-Forwarded-Port":   []string{"443"},
				"X-Forwarded-Proto":  []string{"https"},
			},
		},
		{
			name:       "set xff, host, port, proto, method, uri with non well known port",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{For: true, Host: true, Method: true, Uri: true, Port: "4444", Proto: "https"},
			method:     "POST",
			requestURI: "/foo?bar=baz",
			expected: http.Header{
				"X-Forwarded-For":    []string{"1.2.3.4"},
				"X-Forwarded-Host":   []string{"example.com"},
				"X-Forwarded-Method": []string{"POST"},
				"X-Forwarded-Uri":    []string{"/foo?bar=baz"},
				"X-Forwarded-Port":   []string{"4444"},
				"X-Forwarded-Proto":  []string{"https"},
			},
		},
		{
			name:       "set xff, overwrite host, port, proto",
			remoteAddr: "1.2.3.4:56",
			header: http.Header{
				"X-Forwarded-Host":  []string{"whatever"},
				"X-Forwarded-Port":  []string{"whatever"},
				"X-Forwarded-Proto": []string{"whatever"},
			},
			forwarded: ForwardedHeaders{For: true, Host: true, Port: "443", Proto: "https"},
			expected: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4"},
				"X-Forwarded-Host":  []string{"example.com"},
				"X-Forwarded-Port":  []string{"443"},
				"X-Forwarded-Proto": []string{"https"},
			},
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			r := &http.Request{
				Host:       "example.com",
				RemoteAddr: ti.remoteAddr,
				Header:     ti.header,
				Method:     ti.method,
				RequestURI: ti.requestURI,
			}

			ti.forwarded.Set(r)

			if !reflect.DeepEqual(ti.expected, r.Header) {
				t.Errorf("header mismatch:\n%v", cmp.Diff(ti.expected, r.Header))
			}
		})
	}
}

func TestForwardedHeadersHandler(t *testing.T) {
	for _, ti := range []struct {
		name       string
		remoteAddr string
		exclude    []string
		expected   bool
	}{
		{
			name:       "no exclude",
			remoteAddr: "1.2.3.4:56",
			expected:   true,
		},
		{
			name:       "exclude does not match",
			remoteAddr: "1.2.3.4:56",
			exclude:    []string{"4.5.6.0/24"},
			expected:   true,
		},
		{
			name:       "exclude matches",
			remoteAddr: "1.2.3.4:56",
			exclude:    []string{"1.2.3.4/24"},
			expected:   false,
		},
		{
			name:       "invalid remote address",
			remoteAddr: "invalid",
			expected:   false,
		},
	} {
		t.Run(ti.name, func(t *testing.T) {
			nets, err := ParseCIDRs(ti.exclude)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			delegated := false
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, got := r.Header["X-Forwarded-For"]
				if ti.expected != got {
					t.Errorf("X-Forwarded-For expected: %t, got: %t", ti.expected, got)
				}
				delegated = true
			})

			fh := ForwardedHeadersHandler{ForwardedHeaders{For: true}, nets, h}

			fh.ServeHTTP(nil, &http.Request{Host: "example.com", RemoteAddr: ti.remoteAddr, Header: http.Header{}})

			if !delegated {
				t.Fatalf("delegate handler was not called")
			}
		})
	}
}
