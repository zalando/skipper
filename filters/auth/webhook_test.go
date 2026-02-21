package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

const headerToCopy = "X-Copy-Header"

func TestWebhook(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		token       string
		expected    int
		authorized  bool
		timeout     bool
		copyHeaders bool
	}{{
		msg:         "invalid-token-should-be-unauthorized",
		token:       "invalid-token",
		expected:    http.StatusUnauthorized,
		authorized:  false,
		copyHeaders: true,
	}, {
		msg:         "valid-token-should-be-authorized",
		token:       testToken,
		expected:    http.StatusOK,
		authorized:  true,
		copyHeaders: true,
	}, {
		msg:        "webhook-timeout-should-be-unauthorized",
		token:      testToken,
		expected:   http.StatusUnauthorized,
		authorized: false,
		timeout:    true,
	}, {
		msg:        "invalid-scope-should-be-forbidden",
		token:      testWebhookInvalidScopeToken,
		expected:   http.StatusForbidden,
		authorized: false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// if header has been forwarded, copy to the request so that we can access in the test.
				if r.Header.Get(headerToCopy) != "" {
					w.Header().Set(headerToCopy, r.Header.Get(headerToCopy))
				}
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, "Hello from backend")
			}))
			defer backend.Close()

			d := 100 * time.Millisecond
			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if ti.timeout {
					time.Sleep(2 * d)
				}

				if r.Method != "GET" {
					w.WriteHeader(http.StatusMethodNotAllowed)
					io.WriteString(w, "FAIL - not a GET request")
					return
				}

				// Set header on response that should be copied to the
				// continuing request
				w.Header().Set(headerToCopy, "test")

				tok := r.Header.Get(authHeaderName)
				tok = tok[len(authHeaderPrefix):]
				switch tok {
				case testToken:
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "OK - Got token: "+tok)
					return
				case testWebhookInvalidScopeToken:
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintln(w, "Forbidden - Got token: "+tok)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, "Unauthorized - Got token: "+tok)
			}))
			defer authServer.Close()

			spec := NewWebhook(d)

			args := []any{
				"http://" + authServer.Listener.Addr().String(),
			}

			if ti.copyHeaders {
				args = append(args, headerToCopy)
			}

			_, err := spec.CreateFilter(args)
			if err != nil {
				t.Fatalf("error creating filter for %s: %v", ti.msg, err)
			}

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()

			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Fatalf("failed to create request %v", err)
			}
			req.Header.Set(authHeaderName, authHeaderPrefix+ti.token)

			rsp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("failed to get response: %v", err)
			}
			defer rsp.Body.Close()

			buf := make([]byte, 128)
			var n int
			if n, err = rsp.Body.Read(buf); err != nil && err != io.EOF {
				t.Fatalf("Could not read response body: %v", err)
			}

			if rsp.StatusCode != ti.expected {
				t.Fatalf("unexpected status code: %v != %v %d %s", rsp.StatusCode, ti.expected, n, buf)
			}

			// check that the header was passed forward to the backend request, if it should have been
			if ti.authorized && ti.copyHeaders {
				if rsp.Header.Get(headerToCopy) != "test" {
					t.Errorf("unexpected header value: %v != %v", rsp.Header.Get(headerToCopy), "test")
				}
			} else {
				if rsp.Header.Get(headerToCopy) != "" {
					t.Errorf("unexpected header value: %v != %v", rsp.Header.Get(headerToCopy), "")
				}
			}
		})
	}
}
