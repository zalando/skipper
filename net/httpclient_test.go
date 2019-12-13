package net

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/tracing/tracers/basic"
)

func TestTransport(t *testing.T) {
	tracer, err := basic.InitTracer(nil)
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}

	for _, tt := range []struct {
		name        string
		options     Options
		spanName    string
		bearerToken string
		req         *http.Request
		wantErr     bool
	}{
		{
			name:    "All defaults, with request should have a response",
			req:     httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr: false,
		},
		{
			name: "With opentracing, should have opentracing headers",
			options: Options{
				Tracer: tracer,
			},
			spanName: "myspan",
			req:      httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr:  false,
		},
		{
			name:        "With bearer token request should have a token in the request observed by the endpoint",
			bearerToken: "my-token",
			req:         httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr:     false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := startTestServer(func(r *http.Request) {
				if r.Method != tt.req.Method {
					t.Errorf("wrong request method got: %s, want: %s", r.Method, tt.req.Method)
				}

				if tt.wantErr {
					return
				}

				if tt.spanName != "" && tt.options.Tracer != nil {
					if r.Header.Get("Ot-Tracer-Sampled") == "" ||
						r.Header.Get("Ot-Tracer-Traceid") == "" ||
						r.Header.Get("Ot-Tracer-Spanid") == "" {
						t.Errorf("One of the OT Tracer headers are missing: %v", r.Header)
					}
				}

				if tt.bearerToken != "" {
					if r.Header.Get("Authorization") != "Bearer my-token" {
						t.Errorf("Failed to have a token, but want to have it, got: %v, want: %v", r.Header.Get("Authorization"), "Bearer "+tt.bearerToken)
					}
				}
			})

			defer s.Close()

			quit := make(chan struct{})
			rt := NewTransport(tt.options, quit)
			defer close(quit)

			if tt.spanName != "" {
				rt = WithSpanName(rt, tt.spanName)
			}
			if tt.bearerToken != "" {
				rt = WithBearerToken(rt, tt.bearerToken)
			}

			if tt.req != nil {
				tt.req.URL.Host = s.Listener.Addr().String()
			}
			_, err := rt.RoundTrip(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Transport.RoundTrip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

		})
	}
}

type requestCheck func(*http.Request)

func startTestServer(check requestCheck) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)

		w.Header().Set("X-Test-Response-Header", "response header value")
		w.WriteHeader(http.StatusOK)
	}))
}
