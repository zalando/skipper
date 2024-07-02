package proxy_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
	xtrace "golang.org/x/exp/trace"
)

func TestFlightRecorder(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(http.StatusText(http.StatusMethodNotAllowed)))
			return
		}

		var buf bytes.Buffer
		n, err := io.Copy(&buf, r.Body)
		if err != nil {
			t.Fatalf("Failed to copy data: %v", err)
		}
		if n < 100 {
			t.Fatalf("Failed to write enough data: %d bytes", n)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(http.StatusText(http.StatusCreated)))

	}))
	defer service.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	flightRecorder := xtrace.NewFlightRecorder()
	flightRecorder.Start()

	spec := diag.NewTrace()
	fr := make(filters.Registry)
	fr.Register(spec)

	doc := fmt.Sprintf(`r: * -> trace("20µs") -> "%s"`, backend.URL)
	rr := eskip.MustParse(doc)

	pr := proxytest.WithParams(fr, proxy.Params{
		FlightRecorder:          flightRecorder,
		FlightRecorderTargetURL: service.URL,
	}, rr...)
	defer pr.Close()

	rsp, err := pr.Client().Get(pr.URL)
	if err != nil {
		t.Fatalf("Failed to GET %q: %v", pr.URL, err)
	}
	defer rsp.Body.Close()
	_, err = io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	switch rsp.StatusCode {
	case 200, 201, 204:
		// ok
	default:
		t.Fatalf("Failed to get status OK: %d", rsp.StatusCode)
	}
}
