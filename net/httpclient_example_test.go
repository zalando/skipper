package net_test

import (
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/lightstep/lightstep-tracer-go"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/secrets"
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

	for i := 0; i < 15; i++ {
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

	for i := 0; i < 15; i++ {
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

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
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

	for i := 0; i < 15; i++ {
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

	for i := 0; i < 15; i++ {
		rsp, err := cli.Get(u)
		if err != nil {
			log.Fatalf("Failed to do request: %v", err)
		}
		log.Printf("rsp code: %v", rsp.StatusCode)
		time.Sleep(1 * time.Second)
	}
}
