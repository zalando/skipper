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
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(200)
		w.Write(payload)
	}))
	defer s.Close()

	p := &proxy{makeTestetcdc(s.URL), &http.Client{}}
	r := &http.Request{}
	w := httptest.NewRecorder()
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
