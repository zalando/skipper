package xforward

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

type testItem struct {
	title          string
	requestHeaders map[string]string
}

func getForwardedFor(headerValue string, prepend bool) []string {
	values := strings.Split(headerValue, ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}

	if len(values) == 1 && values[0] == "" {
		return nil
	}

	if !prepend {
		return values
	}

	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}

	return values
}

func createTest(prepend bool, test testItem) func(*testing.T) {
	return func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Received-Forwarded-For", r.Header.Get("X-Forwarded-For"))
			w.Header().Set("X-Received-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		}))
		defer backend.Close()

		var spec filters.Spec
		if prepend {
			spec = NewFirst()
		} else {
			spec = New()
		}

		fr := make(filters.Registry)
		fr.Register(spec)
		proxy := proxytest.New(fr, &eskip.Route{
			Filters: []*eskip.Filter{{
				Name: spec.Name(),
			}},
			Backend: backend.URL,
		})
		defer proxy.Close()

		req, err := http.NewRequest("GET", proxy.URL, nil)
		if err != nil {
			t.Fatal(err)
		}

		req.Host = "www.example.org"
		for name, value := range test.requestHeaders {
			req.Header.Add(name, value)
		}

		client := &http.Client{}
		rsp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			t.Fatalf(
				"Unexpected status code, got: %d, expected: %d",
				rsp.StatusCode,
				http.StatusOK,
			)
		}

		reqXFor := getForwardedFor(req.Header.Get("X-Forwarded-For"), prepend)
		rspXFor := getForwardedFor(rsp.Header.Get("X-Received-Forwarded-For"), prepend)
		if len(rspXFor) != len(reqXFor)+1 {
			t.Fatal("Failed to add X-Forwarded-For header.")
		}

		for i := range reqXFor {
			if rspXFor[i] != reqXFor[i] {
				t.Fatalf(
					"Failed to keep the received X-Forwarded-For header, got: '%s'.",
					rsp.Header.Get("X-Received-Forwarded-For"),
				)
			}
		}

		expectedHost := "www.example.org"
		if rsp.Header.Get("X-Received-Forwarded-Host") != expectedHost {
			t.Fatalf(
				"Failed to set the X-Forwarded-Host header, got '%s', expected: '%s'.",
				rsp.Header.Get("X-Received-Forwarded-Host"),
				expectedHost,
			)
		}
	}
}

func testItems() []testItem {
	return []testItem{{
		title: "set",
	}, {
		title: "add",
		requestHeaders: map[string]string{
			"X-Forwarded-For": "10.0.1.1, 10.0.2.1",
		},
	}, {
		title: "override forwarded host",
		requestHeaders: map[string]string{
			"X-Forwarded-Host": "foo.example.org",
		},
	}}
}

func TestXForward(t *testing.T) {
	for _, test := range testItems() {
		t.Run(test.title, createTest(false, test))
	}
}

func TestXForwardFirst(t *testing.T) {
	for _, test := range testItems() {
		t.Run(test.title, createTest(true, test))
	}
}
