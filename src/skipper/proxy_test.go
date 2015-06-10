package main

import "bytes"
import "testing"
import "net/http"
import "net/http/httptest"
import "strconv"

type testetcdc struct {
	s settings
	c chan settings
}

func makeTestetcdc(url string) etcdc {
	ec := &testetcdc{
		&settingsStruct{
			backends: []backend{
				backend{
					typ: ephttp,
					servers: []server{
						server{url: url}}}},
			frontends:  []frontend{},
			middleware: []middleware{}},
		make(chan settings)}
	go func() {
		for {
			ec.c <- ec.s
		}
	}()
	return ec
}

func (ec *testetcdc) getSettings() <-chan settings {
	return ec.c
}

func TestGetRoundtrip(t *testing.T) {
	payload := []byte("Hello World!")

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("wrong request method")
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(200)
		w.Write(payload)
	}))
	defer s.Close()

	r := &http.Request{
		Method: "GET",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()
	p := makeProxy(makeTestetcdc(s.URL))
	p.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Error("wrong status")
	}

	if ct, ok := w.Header()["Content-Type"]; !ok || ct[0] != "text/plain" {
		t.Error("wrong content type")
	}

	if cl, ok := w.Header()["Content-Length"]; !ok || cl[0] != strconv.Itoa(len(payload)) {
		t.Error("wrong content length")
	}

	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Error("wrong content")
	}
}

func TestPostRoundtrip(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong request method")
		}

		if th, ok := r.Header["X-Test-Header"]; !ok || th[0] != "test value" {
			t.Error("wrong request header")
		}

		w.Header().Set("Location", "https://www.zalando.de")
		w.WriteHeader(303)
	}))
	defer s.Close()

	r := &http.Request{
		Method: "POST",
		Header: http.Header{"X-Test-Header": []string{"test value"}}}
	w := httptest.NewRecorder()
	p := makeProxy(makeTestetcdc(s.URL))
	p.ServeHTTP(w, r)

	if w.Code != 303 {
		println(w.Code)
		t.Error("wrong status")
	}

	if cl, ok := w.Header()["Location"]; !ok || cl[0] != "https://www.zalando.de" {
		t.Error("wrong location header")
	}

	if w.Body.Len() != 0 {
		t.Error("wrong content")
	}
}
