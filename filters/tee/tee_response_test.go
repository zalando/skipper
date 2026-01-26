package tee

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestTeeResponseEndToEndBody(t *testing.T) {
	s := "hello"

	shadowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read shadow request: %v", err)
		}

		if s != string(b) {
			t.Fatalf("Failed to get the shadow request %q != %q", s, string(b))
		}

		r.Body.Close()
	}))
	defer shadowBackend.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		w.Write([]byte(s))
	}))
	defer backend.Close()

	routeStr := fmt.Sprintf(`route1: * -> teeResponse("%v") -> "%v";`, shadowBackend.URL, backend.URL)

	route := eskip.MustParse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTeeResponse(Options{}))
	p := proxytest.New(registry, route...)
	defer p.Close()

	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		t.Error(err)
	}

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}
	res := string(b)
	if res != s {
		t.Fatalf("Failed to get client result: %q != %q", res, s)
	}

	rsp.Body.Close()
}

func TestTeeResponseNoResponseBody(t *testing.T) {
	shadowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if n := len(b); n != 0 {
			t.Fatalf("Failed to get no body, got: %d", n)
		}

		r.Body.Close()
	}))
	defer shadowBackend.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		w.WriteHeader(200)
		w.Write([]byte(""))
	}))
	defer backend.Close()

	routeStr := fmt.Sprintf(`route1: * -> teeResponse("%v") -> "%v";`, shadowBackend.URL, backend.URL)

	route := eskip.MustParse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTeeResponse(Options{}))
	p := proxytest.New(registry, route...)
	defer p.Close()

	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		t.Error(err)
	}

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}
	res := string(b)
	if res != "" {
		t.Fatalf("Failed to get client result: %q != %q", res, "")
	}

	rsp.Body.Close()
}

func TestTeeResponseFailingShadow(t *testing.T) {
	s := "hello"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		w.WriteHeader(200)
		w.Write([]byte(s))
	}))
	defer backend.Close()

	routeStr := fmt.Sprintf(`route1: * -> teeResponse("%v") -> "%v";`, "http://localhost:34125", backend.URL)

	route := eskip.MustParse(routeStr)
	registry := make(filters.Registry)
	registry.Register(NewTeeResponse(Options{}))
	p := proxytest.New(registry, route...)
	defer p.Close()

	req, err := http.NewRequest("GET", p.URL, nil)
	if err != nil {
		t.Error(err)
	}

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}
	res := string(b)
	if res != s {
		t.Fatalf("Failed to get client result: %q != %q", res, s)
	}

	rsp.Body.Close()
}
