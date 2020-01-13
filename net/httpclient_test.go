package net

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/tracing/tracers/basic"
)

var testToken = []byte("mytoken1")

type testSecretsReader struct {
	h map[string][]byte
}

func newTestSecretsReader(m map[string][]byte) *testSecretsReader {
	return &testSecretsReader{
		h: m,
	}
}

func (*testSecretsReader) Close() {}
func (tsr *testSecretsReader) GetSecret(k string) ([]byte, bool) {
	b, ok := tsr.h[k]
	return b, ok
}

func TestClient(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}

	for _, tt := range []struct {
		name      string
		options   Options
		tokenFile string
		wantErr   bool
	}{
		{
			name:    "All defaults, with request should have a response",
			wantErr: false,
		},
		{
			name: "With tracer",
			options: Options{
				Tracer: tracer,
			},
			wantErr: false,
		},
		{
			name: "With tracer and span name",
			options: Options{
				Tracer:                  tracer,
				OpentracingComponentTag: "mytag",
				OpentracingSpanName:     "foo",
			},
			wantErr: false,
		},
		{
			name:      "With token",
			options:   Options{},
			tokenFile: "token",
			wantErr:   false,
		},
		{
			name: "With static secrets reader",
			options: Options{
				SecretsReader: secrets.StaticSecret(testToken),
			},
			wantErr: false,
		},
		{
			name: "With static delegate secrets reader",
			options: Options{
				SecretsReader: secrets.NewStaticDelegateSecret(newTestSecretsReader(map[string][]byte{
					"key": testToken,
				}), "key"),
			},
			wantErr: false,
		},
		{
			name: "With HostSecret secrets reader",
			options: Options{
				SecretsReader: secrets.NewHostSecret(
					newTestSecretsReader(map[string][]byte{
						"key": testToken,
					}),
					map[string]string{
						"127.0.0.1": "key",
					},
				),
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := startTestServer(func(r *http.Request) {
				if tt.options.OpentracingSpanName != "" && tt.options.Tracer != nil {
					if r.Header.Get("Ot-Tracer-Sampled") == "" ||
						r.Header.Get("Ot-Tracer-Traceid") == "" ||
						r.Header.Get("Ot-Tracer-Spanid") == "" {
						t.Errorf("One of the OT Tracer headers are missing: %v", r.Header)
					}
				}

				if tt.tokenFile != "" || tt.options.SecretsReader != nil {
					switch auth := r.Header.Get("Authorization"); auth {
					case "Bearer " + string(testToken):
						if tt.wantErr {
							t.Error("Want error and not an Authorization header")
						}
					default:
						if !tt.wantErr {
							t.Errorf("Wrong Authorization header '%s'", auth)
						}
					}
				} else if r.Header.Get("Authorization") != "" {
					t.Errorf("Client should not have an authorization header: %s", r.Header.Get("Authorization"))
				}
			})
			defer s.Close()

			if tt.tokenFile != "" {
				dir, err := ioutil.TempDir("/tmp", "skipper-httpclient-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(dir) // clean up
				tokenFile := filepath.Join(dir, tt.tokenFile)
				if err := ioutil.WriteFile(tokenFile, []byte(testToken), 0600); err != nil {
					t.Fatalf("Failed to create token file: %v", err)
				}
				tt.options.BearerTokenFile = tokenFile
			}

			cli := NewClient(tt.options)
			defer cli.Close()

			u := "http://" + s.Listener.Addr().String() + "/"

			_, err = cli.Get(u)
			if err != nil {
				t.Errorf("Failed to do GET request error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			_, err = cli.Head(u)
			if err != nil {
				t.Errorf("Failed to do HEAD request error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			_, err = cli.Post(u, "", nil)
			if err != nil {
				t.Errorf("Failed to do POST request error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			_, err = cli.PostForm(u, url.Values{})
			if err != nil {
				t.Errorf("Failed to do POST form request error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			req, err := http.NewRequest("DELETE", u, nil)
			if err != nil {
				t.Errorf("Failed to create DELETE request: %v", err)
			}
			_, err = cli.Do(req)
			if err != nil {
				t.Errorf("Failed to do DELETE request error = %v, wantErr %v", err, tt.wantErr)
				return
			}

		})
	}
}

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
			bearerToken: string(testToken),
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
					if r.Header.Get("Authorization") != "Bearer "+string(testToken) {
						t.Errorf("Failed to have a token, but want to have it, got: %v, want: %v", r.Header.Get("Authorization"), "Bearer "+tt.bearerToken)
					}
				}
			})

			defer s.Close()

			rt := NewTransport(tt.options)
			defer rt.Close()

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
