package net

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/google/go-cmp/cmp"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/tracing/tracers/basic"
	"github.com/zalando/skipper/tracing/tracingtest"
)

var testToken = []byte("mytoken1")

var globalServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

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
	defer tracer.Close()

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
			name: "Idle conn timeout",
			options: Options{
				Timeout: 3 * time.Second,
			},
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
		{
			name: "With Transport",
			options: Options{
				Transport: &http.Transport{
					ResponseHeaderTimeout: 5 * time.Second,
				},
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
				dir, err := os.MkdirTemp("/tmp", "skipper-httpclient-test")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(dir) // clean up
				tokenFile := filepath.Join(dir, tt.tokenFile)
				if err := os.WriteFile(tokenFile, []byte(testToken), 0600); err != nil {
					t.Fatalf("Failed to create token file: %v", err)
				}
				tt.options.BearerTokenFile = tokenFile
			}

			cli := NewClient(tt.options)
			if cli == nil {
				t.Fatal("NewClient returned nil")
			}
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
	mtracer := tracingtest.NewTracer()
	tracer, err := basic.InitTracer(nil)
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}
	defer tracer.Close()

	for _, tt := range []struct {
		name                 string
		options              Options
		spanName             string
		bearerToken          string
		req                  *http.Request
		wantErr              bool
		checkRequestOnServer func(*http.Request) error
		checkRequest         func(*http.Request) error
		checkResponse        func(*http.Response) error
	}{
		{
			name:    "All defaults, with request should have a response",
			req:     httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr: false,
		},
		{
			name: "Transport, with request should have a response",
			options: Options{
				Transport: &http.Transport{
					ResponseHeaderTimeout: 5 * time.Second,
				},
			},
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
		{
			name: "With hooks, should have request header and response changed",
			options: Options{
				BeforeSend: func(req *http.Request) {
					if req != nil {
						req.Header.Set("X-Foo", "bar")
					}
				},
				AfterResponse: func(rsp *http.Response, err error) {
					if rsp != nil {
						rsp.StatusCode = 255
					}
				},
			},
			req:     httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr: false,
			checkRequestOnServer: func(req *http.Request) error {
				if v := req.Header.Get("X-Foo"); v != "bar" {
					return fmt.Errorf(`failed to patch request want "X-Foo": "bar", but got: %s`, v)
				}
				return nil
			},
			checkResponse: func(rsp *http.Response) error {
				if rsp.StatusCode != 255 {
					return fmt.Errorf("failed to get status code 255, got: %d", rsp.StatusCode)
				}
				return nil
			},
		},
		{
			name: "With hooks and opentracing, should have request header and response changed",
			options: Options{
				Tracer: mtracer,
				BeforeSend: func(req *http.Request) {
					if req != nil {
						if span := opentracing.SpanFromContext(req.Context()); span != nil {
							span.SetTag(string(ext.PeerService), "my-app")
							*req = *req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
							return
						}
					}
				},
				AfterResponse: func(rsp *http.Response, err error) {
					if rsp != nil {
						if span := opentracing.SpanFromContext(rsp.Request.Context()); span != nil {
							span.SetTag("my.status", 255)
							*rsp.Request = *rsp.Request.WithContext(opentracing.ContextWithSpan(rsp.Request.Context(), span))
							return
						}
					}
				},
			},
			spanName: "myspan",
			req:      httptest.NewRequest("GET", "http://example.com/", nil),
			wantErr:  false,
			checkRequest: func(req *http.Request) error {
				if span := opentracing.SpanFromContext(req.Context()); span != nil {
					peerService := mtracer.FinishedSpans()[0].Tags()[string(ext.PeerService)]
					if peerService != "my-app" {
						return fmt.Errorf(`failed to get Tag %s value: "my-app", got %q`, ext.PeerService, peerService)
					}
					return nil
				}
				return fmt.Errorf("failed get span from request")
			},
			checkResponse: func(rsp *http.Response) error {
				if span := opentracing.SpanFromContext(rsp.Request.Context()); span != nil {
					status := mtracer.FinishedSpans()[0].Tags()["my.status"]
					if status != 255 {
						return fmt.Errorf(`failed to get Tag "my.status" value: "255", got %d`, status)
					}
					return nil
				}
				return fmt.Errorf("failed get span from request")
			},
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

				if tt.spanName != "" {
					if tt.options.Tracer == tracer {
						if r.Header.Get("Ot-Tracer-Sampled") == "" ||
							r.Header.Get("Ot-Tracer-Traceid") == "" ||
							r.Header.Get("Ot-Tracer-Spanid") == "" {
							t.Errorf("One of the OT Tracer headers are missing: %v", r.Header)
						}
					}
				}
				if tt.bearerToken != "" {
					if r.Header.Get("Authorization") != "Bearer "+string(testToken) {
						t.Errorf("Failed to have a token, but want to have it, got: %v, want: %v", r.Header.Get("Authorization"), "Bearer "+tt.bearerToken)
					}
				}

				if tt.checkRequestOnServer != nil {
					if err := tt.checkRequestOnServer(r); err != nil {
						t.Errorf("Failed to check request: %v", err)
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
			rsp, err := rt.RoundTrip(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Transport.RoundTrip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkRequest != nil {
				if err := tt.checkRequest(rsp.Request); err != nil {
					t.Errorf("Failed to check request: %v", err)
				}
			}

			if tt.checkResponse != nil {
				if err := tt.checkResponse(rsp); err != nil {
					t.Errorf("Failed to check response: %v", err)
				}
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

func TestClientClosesIdleConnections(t *testing.T) {
	noleak.Check(t)

	cli := NewClient(Options{})
	defer cli.Close()

	rsp, err := cli.Get(globalServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %s", rsp.Status)
	}
	rsp.Body.Close()
}

func TestClientOTSpan(t *testing.T) {
	tracer := tracingtest.NewTracer()
	spanName := "foo"
	componentTagValue := "my-component"
	opt := Options{
		Tracer:                  tracer,
		OpentracingComponentTag: componentTagValue,
		OpentracingSpanName:     spanName,
	}
	requestString := `{"foo": "bar", "int": 5}`
	responseString := "Everything is fine"

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body by server: %v", err)
		}
		if s := string(srvBody); s != requestString {
			t.Errorf("Failed to get the expected request body: %q", cmp.Diff(requestString, s))
		}
		r.Body.Close()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseString))

	}))
	defer backend.Close()

	cli := NewClient(opt)
	if cli == nil {
		t.Fatal("NewClient returned nil")
	}
	defer cli.Close()
	defer cli.CloseIdleConnections()

	u := "http://" + backend.Listener.Addr().String() + "/"

	body := bytes.NewBufferString(requestString)
	rsp, err := cli.Post(u, textproto.CanonicalMIMEHeaderKey("application/json"), body)
	if err != nil {
		t.Fatalf("Failed to do GET request error = %v", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get expected (200) response: %d", rsp.StatusCode)
	}
	rspBody, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("no body send, but expected: %q, got: %q", responseString, string(rspBody))
	}

	span := tracer.FindSpan(spanName)
	if span == nil {
		t.Fatalf("Failed to find span %q", spanName)
	}
	verifyTag(t, span, "http.method", "POST")
	verifyTagHasNonZeroValue(t, span, "connect")
}

func verifyTag(t *testing.T, span *tracingtest.MockSpan, name string, expected interface{}) {
	t.Helper()
	if got := span.Tag(name); got != expected {
		t.Errorf("unexpected %q tag value: %q != %q", name, got, expected)
	}
}

func verifyTagHasNonZeroValue(t *testing.T, span *tracingtest.MockSpan, name string) {
	t.Helper()
	if got := span.Tag(name); got != nil && got != "" {
		t.Errorf("unexpected %q tag value: %q", name, got)
	}
}
