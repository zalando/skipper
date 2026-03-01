package logging

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWrites(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}

	body := "Hello, world!"
	w.Write([]byte(body))
	back := rr.Body.String()

	if back != body {
		t.Error("failed to write body")
	}

	if w.bytes != int64(len(body)) {
		t.Error("failed to count bytes")
	}
}

func TestWritesAndStoresStatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.WriteHeader(http.StatusTeapot)

	if rr.Code != http.StatusTeapot {
		t.Error("failed to write status code")
	}

	if w.GetCode() != http.StatusTeapot {
		t.Error("failed to get status code")
	}

	w.WriteHeader(0)
	if w.GetCode() != http.StatusOK {
		t.Errorf("Failed to get default status %d", http.StatusOK)
	}

}

func TestReturnsUnderlyingHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.Header().Set("X-Test-Header", "test-value")
	if rr.Header().Get("X-Test-Header") != "test-value" {
		t.Error("failed to set the header")
	}
}

func TestFlushesPartialPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &LoggingWriter{writer: rr}
	w.Write([]byte("Hello, world!"))
	w.Flush()
	if !rr.Flushed {
		t.Error("failed to flush underlying writer")
	}
}

func TestHijack(t *testing.T) {
	t.Run("Failing hijack test", func(t *testing.T) {
		rr := httptest.NewRecorder()
		lw := NewLoggingWriter(rr)
		n, err := lw.Write([]byte("Hello, world!"))
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		if m := lw.GetBytes(); m != int64(n) {
			t.Fatalf("Failed to get correct length of bytes want %d, got: %d", n, m)
		}
		_, _, err = lw.Hijack()
		if err == nil {
			t.Fatal("Failed to get hijack error")
		}
	})

	t.Run("Working hijacked connection", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {

			w := NewLoggingWriter(rw)
			w.WriteHeader(http.StatusSwitchingProtocols)

			conn, bufrw, err := w.Hijack()
			if err != nil {
				t.Fatalf("Failed to get hijacker: %v", err)
			}
			defer conn.Close()

			for {
				s, err := bufrw.ReadString('\n')
				if err != nil {
					return
				}

				var resp string
				if strings.Compare(s, "ping\n") == 0 {
					resp = "pong\n"
				} else {
					resp = "bad\n"
				}

				_, err = bufrw.WriteString(resp)
				if err != nil {
					return
				}
				err = bufrw.Flush()
				if err != nil {
					return
				}
			}
		}))
		defer backend.Close()

		req, err := http.NewRequest("GET", backend.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header = getValidUpgradeHeaders()

		u, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("Failed to parse url: %v", err)
		}

		conn, err := net.Dial("tcp", u.Host)
		if err != nil {
			t.Fatalf("Failed to dial to %s: %v", u.Host, err)
		}

		err = req.Write(conn)
		if err != nil {
			t.Fatalf("Failed to write request to conn: %v", err)
		}

		reader := bufio.NewReader(conn)
		rsp, err := http.ReadResponse(reader, req)
		if err != nil {
			t.Fatalf("Failed to read response from conn: %v", err)
		}
		if rsp.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("Failed to switch protocols: %d", rsp.StatusCode)
		}

		// we have an upgraded connection and we ping/pong through it
		// to test the successful Hijacked connection
		_, err = conn.Write([]byte("ping\n"))
		if err != nil {
			t.Fatalf("Failed to write upgraded connection: %v", err)
		}

		pong, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("Failed to read from upgraded connection: %v", err)
		}
		if pong != "pong\n" {
			t.Fatalf("Failed to get the correct pong, got: %q", pong)
		}
	})

}

func getValidUpgradeHeaders() http.Header {
	//Connection:[Upgrade] Upgrade:[SPDY/3.1]
	//prot := "HTTP/2.0, SPDY/3.1"
	prot := "SPDY/3.1"
	header := http.Header{}
	header.Add("Connection", "Upgrade")
	header.Add("Upgrade", prot)
	return header
}
