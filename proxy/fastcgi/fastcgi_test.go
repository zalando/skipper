package fastcgi

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"strconv"
	"testing"

	"github.com/zalando/skipper/logging"
)

func TestFastCgi(t *testing.T) {
	payload := []byte("Hello, World!")
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Response-Header", "response header value")

		if len(payload) <= 0 {
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)

		w.Write(payload)
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer l.Close()

	go fcgi.Serve(l, nil)

	u, _ := url.ParseRequestURI("http://www.example.org/hello")

	rt, err := NewRoundTripper(&logging.DefaultLog{}, l.Addr().String(), "index.php")
	if err != nil {
		t.Errorf("could not create roundtripper: %v", err)

		return
	}

	r := &http.Request{
		URL:    u,
		Method: "GET",
		Proto:  "HTTP/1.0",
	}

	response, err := rt.RoundTrip(r)
	if err != nil {
		t.Errorf("could not create roundtrip: %v", err)

		return
	}
	b, err := io.ReadAll(response.Body)
	if err != nil {
		t.Error(err)

		return
	}

	if response.StatusCode != http.StatusOK || !bytes.Equal(b, payload) {
		t.Error("wrong routing 1")
	}
}
