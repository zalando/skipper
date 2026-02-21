package net

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
		tls        bool
		localAddr  string
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
		{
			name:       "proto auto detect with TLS",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{Proto: "auto"},
			tls:        true,
			expected: http.Header{
				"X-Forwarded-Proto": []string{"https"},
			},
		},
		{
			name:       "proto auto detect without TLS",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{Proto: "auto"},
			tls:        false,
			expected: http.Header{
				"X-Forwarded-Proto": []string{"http"},
			},
		},
		{
			name:       "port auto detect",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{Port: "auto"},
			localAddr:  "0.0.0.0:8443",
			expected: http.Header{
				"X-Forwarded-Port": []string{"8443"},
			},
		},
		{
			name:       "proto and port auto detect together",
			remoteAddr: "1.2.3.4:56",
			header:     http.Header{},
			forwarded:  ForwardedHeaders{Proto: "auto", Port: "auto"},
			tls:        true,
			localAddr:  "0.0.0.0:443",
			expected: http.Header{
				"X-Forwarded-Proto": []string{"https"},
				"X-Forwarded-Port":  []string{"443"},
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

			if ti.tls {
				r.TLS = &tls.ConnectionState{}
			}

			if ti.localAddr != "" {
				addr, _ := net.ResolveTCPAddr("tcp", ti.localAddr)
				ctx := context.WithValue(r.Context(), http.LocalAddrContextKey, addr)
				r = r.WithContext(ctx)
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

func TestContentLengthHeaderHandler(t *testing.T) {
	for _, tt := range []struct {
		name          string
		bodySize      int
		max           int64
		want          int
		wantDelegated bool
	}{
		{
			name:          "test GET",
			max:           10,
			want:          http.StatusOK,
			wantDelegated: true,
		},
		{
			name:          "test POST",
			bodySize:      10,
			max:           1000,
			want:          http.StatusOK,
			wantDelegated: true,
		},
		{
			name:          "test POST max",
			bodySize:      10,
			max:           10,
			want:          http.StatusOK,
			wantDelegated: true,
		},
		{
			name:          "test POST too large",
			bodySize:      100,
			max:           10,
			want:          http.StatusRequestEntityTooLarge,
			wantDelegated: false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/foo", nil)
			if err != nil {
				t.Fatalf("Failed to creae GET request: %v", err)
			}
			if tt.bodySize != 0 {
				req, err = http.NewRequest("POST", "http://example.com/foo", bytes.NewBufferString(strings.Repeat("A", tt.bodySize)))
				if err != nil {
					t.Fatalf("Failed to creae POST request: %v", err)
				}
			}

			delegated := false
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				delegated = true
				if !tt.wantDelegated {
					t.Fatalf("Failed to stop request delegation")
				}
			})

			ch := ContentLengthHeadersHandler{
				Max:     tt.max,
				Handler: h,
			}

			recorder := httptest.NewRecorder()

			ch.ServeHTTP(recorder, req)

			if delegated != tt.wantDelegated {
				t.Fatalf("Failed to get delegation as expected: %v != %v", delegated, tt.wantDelegated)
			}

			if recorder.Code != tt.want {
				t.Fatalf("Failed to get status code as expected: %v != %v", recorder.Code, tt.want)
			}
		})
	}

}
