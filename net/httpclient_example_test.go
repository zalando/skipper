package net_test

import (
	"fmt"
	"log"
	stdlibnet "net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/lightstep/lightstep-tracer-go"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func ExampleTransport() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	cli := net.NewTransport(net.Options{
		Tracer: tracer,
	})
	defer cli.Close()
	cli = net.WithSpanName(cli, "myspan")
	cli = net.WithBearerToken(cli, "mytoken")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	rsp, err := cli.RoundTrip(req)
	if err != nil {
		log.Fatalf("Failed to do request: %v", err)
	}
	log.Printf("rsp code: %v", rsp.StatusCode)
}

func ExampleClient() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	cli := net.NewClient(net.Options{
		Tracer:                     tracer,
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
		IdleConnTimeout:            2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_withTransport() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	d := stdlibnet.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	f := d.DialContext

	cli := net.NewClient(net.Options{
		Transport: &http.Transport{
			IdleConnTimeout: 10 * time.Second,
			DialContext:     f,
		},
		Tracer:                     tracer,
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
	})

	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_fileSecretsReader() {
	tracer := lightstep.NewTracer(lightstep.Options{})

	sp := secrets.NewSecretPaths(10 * time.Second)
	if err := sp.Add("/tmp/bar.token"); err != nil {
		log.Fatalf("failed to read secret: %v", err)
	}

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader:           sp,
		IdleConnTimeout:         2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_staticSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")
	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader:           secrets.StaticSecret(sec),
		IdleConnTimeout:         2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

type customTracer struct {
	opentracing.Tracer
}

func (t *customTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	span := t.Tracer.StartSpan(operationName, opts...)
	span.SetTag("customtag", "test")
	return span
}

func ExampleClient_customTracer() {
	mockTracer := tracingtest.NewTracer()
	cli := net.NewClient(net.Options{
		Tracer:              &customTracer{mockTracer},
		OpentracingSpanName: "clientSpan",
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	cli.Get("http://" + srv.Listener.Addr().String() + "/")
	fmt.Printf("customtag: %s", mockTracer.FinishedSpans()[0].Tags()["customtag"])

	// Output:
	// customtag: test
}

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

func ExampleClient_staticDelegateSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader: secrets.NewStaticDelegateSecret(
			newTestSecretsReader(
				map[string][]byte{
					"key": sec,
				},
			),
			"key",
		),
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_hostSecret() {
	tracer := lightstep.NewTracer(lightstep.Options{})
	sec := []byte("mysecret")

	cli := net.NewClient(net.Options{
		Tracer:                  tracer,
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		SecretsReader: secrets.NewHostSecret(
			newTestSecretsReader(
				map[string][]byte{
					"key": sec,
				},
			),
			map[string]string{
				"127.0.0.1": "key",
			},
		),
		IdleConnTimeout: 2 * time.Second,
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Authorization: %s", r.Header.Get("Authorization"))
		log.Printf("Ot-Tracer-Sampled: %s", r.Header.Get("Ot-Tracer-Sampled"))
		log.Printf("Ot-Tracer-Traceid: %s", r.Header.Get("Ot-Tracer-Traceid"))
		log.Printf("Ot-Tracer-Spanid: %s", r.Header.Get("Ot-Tracer-Spanid"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u := "http://" + srv.Listener.Addr().String() + "/"

	for range 15 {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}

func ExampleClient_withBeforeSendHook() {
	mockTracer := tracingtest.NewTracer()
	peerService := "my-peer-service"
	cli := net.NewClient(net.Options{
		Tracer:                  &customTracer{mockTracer},
		OpentracingComponentTag: "testclient",
		OpentracingSpanName:     "clientSpan",
		IdleConnTimeout:         2 * time.Second,
		BeforeSend: func(req *http.Request) {
			req.Header.Set("X-Foo", "qux")
			if span := opentracing.SpanFromContext(req.Context()); span != nil {
				logrus.Println("BeforeSend: found span")
				span.SetTag(string(ext.PeerService), peerService)
			} else {
				logrus.Println("BeforeSend: no span found")
			}
		},
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("X-Foo: %s\n", r.Header.Get("X-Foo"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli.Get("http://" + srv.Listener.Addr().String() + "/")
	fmt.Printf("request tag %q set to %q", string(ext.PeerService), mockTracer.FinishedSpans()[0].Tags()[string(ext.PeerService)])

	// Output:
	// X-Foo: qux
	// request tag "peer.service" set to "my-peer-service"
}

func ExampleClient_withAfterResponseHook() {
	mockTracer := tracingtest.NewTracer()
	cli := net.NewClient(net.Options{
		Tracer:                     &customTracer{mockTracer},
		OpentracingComponentTag:    "testclient",
		OpentracingSpanName:        "clientSpan",
		BearerTokenRefreshInterval: 10 * time.Second,
		BearerTokenFile:            "/tmp/foo.token",
		IdleConnTimeout:            2 * time.Second,
		AfterResponse: func(rsp *http.Response, err error) {
			if span := opentracing.SpanFromContext(rsp.Request.Context()); span != nil {
				span.SetTag("status.code", rsp.StatusCode)
				if err != nil {
					span.SetTag("error", err.Error())
				}
			}
			rsp.StatusCode = 255
		},
	})
	defer cli.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rsp, err := cli.Get("http://" + srv.Listener.Addr().String() + "/")
	if err != nil {
		log.Fatalf("Failed to get: %v", err)
	}

	fmt.Printf("response code: %d\n", rsp.StatusCode)
	fmt.Printf("span status.code: %d", mockTracer.FinishedSpans()[0].Tags()["status.code"])

	// Output:
	// response code: 255
	// span status.code: 200
}
