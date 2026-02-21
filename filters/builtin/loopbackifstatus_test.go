package builtin

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestLoopbackIfStatusCreateFilter(t *testing.T) {
	spec := NewLoopbackIfStatus()

	// Valid arguments
	f, err := spec.CreateFilter([]any{401, "/new-path"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if f == nil {
		t.Error("expected filter, got nil")
	}

	// Invalid status code type
	_, err = spec.CreateFilter([]any{"401", "/new-path"})
	if err == nil {
		t.Error("expected error for invalid status code type")
	}

	// Invalid status code value
	_, err = spec.CreateFilter([]any{99, "/new-path"})
	if err == nil {
		t.Error("expected error for status code out of range")
	}

	// Invalid path type
	_, err = spec.CreateFilter([]any{401, 123})
	if err == nil {
		t.Error("expected error for invalid path type")
	}

	// Not enough arguments
	_, err = spec.CreateFilter([]any{401})
	if err == nil {
		t.Error("expected error for missing path argument")
	}
}

func TestLoopbackIfStatus(t *testing.T) {

	br := eskip.MustParse(`LOOPBACK_404: Path("/loopback-404") -> inlineContent("404 Not Found") -> <shunt>`)
	br = append(br, eskip.MustParse(`LOOPBACK_500: Path("/loopback-500") -> inlineContent("500 Internal Error") -> <shunt>`)...)

	for _, test := range []struct {
		title           string
		routes          string
		expectedStatus  int
		expectedContent string
	}{{
		title: "404 loopback",
		routes: `*
			-> loopbackIfStatus(404, "/loopback-404")
			-> <shunt>`,
		expectedStatus:  200,
		expectedContent: "404 Not Found",
	}, {
		title: "500 loopback",
		routes: `*
			-> loopbackIfStatus(500, "/loopback-500")
			-> status(500)
			-> <shunt>`,
		expectedStatus:  200,
		expectedContent: "500 Internal Error",
	}} {
		t.Run(test.title, func(t *testing.T) {
			r := append(br, eskip.MustParse(test.routes)...)

			p := proxytest.New(MakeRegistry(), r...)
			defer p.Close()

			rsp, err := http.Get(p.URL)
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != test.expectedStatus {
				t.Error("invalid status received")
				t.Log("got:     ", rsp.StatusCode)
				t.Log("expected:", test.expectedStatus)
			}

			if rsp.Header.Get("Content-Length") !=
				strconv.Itoa(len(test.expectedContent)) {
				t.Error("invalid content length received")
				t.Log("got:     ", rsp.Header.Get("Content-Length"))
				t.Log("expected:", len(test.expectedContent))
			}

			b, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Error(err)
				return
			}

			if string(b) != test.expectedContent {
				t.Error("invalid content received")
				t.Log("got:     ", string(b))
				t.Log("expected:", test.expectedContent)
			}
		})
	}
}
