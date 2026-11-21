package fastcgi

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"reflect"
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

func TestFastCgiDropsProxyHeader(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go fcgi.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("Proxy")))
	}))

	rt, err := NewRoundTripper(&logging.DefaultLog{}, l.Addr().String(), "index.php")
	if err != nil {
		t.Fatalf("could not create roundtripper: %v", err)
	}

	u, _ := url.ParseRequestURI("http://www.example.org/")
	r := &http.Request{
		URL:    u,
		Method: "GET",
		Proto:  "HTTP/1.0",
		Header: http.Header{"Proxy": []string{"http://attacker.example:3128"}},
	}

	response, err := rt.RoundTrip(r)
	if err != nil {
		t.Fatalf("could not create roundtrip: %v", err)
	}
	b, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(b) != 0 {
		t.Errorf("Proxy header passed to FastCGI application as HTTP_PROXY: %q", b)
	}
}

func TestFastCgiDropsUnderscoreHeaders(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go fcgi.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", r.Header.Get("X-Auth-User"), r.Header.Get("X-Request-Id"))
	}))

	for _, tt := range []struct {
		name   string
		header http.Header
		want   string
	}{{
		name:   "underscore only",
		header: http.Header{"X_auth_user": []string{"admin"}},
		want:   "|",
	}, {
		name:   "underscore and dash",
		header: http.Header{"X-Auth-User": []string{"alice"}, "X_auth_user": []string{"admin"}},
		want:   "alice|",
	}, {
		name:   "dash only",
		header: http.Header{"X-Auth-User": []string{"alice"}},
		want:   "alice|",
	}, {
		name:   "unrelated header kept",
		header: http.Header{"X-Request-Id": []string{"42"}},
		want:   "|42",
	}, {
		name:   "unrelated header kept next to dropped one",
		header: http.Header{"X-Request-Id": []string{"42"}, "X_auth_user": []string{"admin"}},
		want:   "|42",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			for range 20 {
				rt, err := NewRoundTripper(&logging.DefaultLog{}, l.Addr().String(), "index.php")
				if err != nil {
					t.Fatalf("could not create roundtripper: %v", err)
				}

				u, _ := url.ParseRequestURI("http://www.example.org/")
				r := &http.Request{
					URL:    u,
					Method: "GET",
					Proto:  "HTTP/1.0",
					Header: tt.header.Clone(),
				}

				response, err := rt.RoundTrip(r)
				if err != nil {
					t.Fatalf("could not create roundtrip: %v", err)
				}
				b, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}

				if string(b) != tt.want {
					t.Fatalf("got %q, want %q", b, tt.want)
				}

				if !reflect.DeepEqual(r.Header, tt.header) {
					t.Fatalf("round tripper modified request header: got %v, want %v", r.Header, tt.header)
				}
			}
		})
	}
}
