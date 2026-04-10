package retry

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestRetry(t *testing.T) {
	for _, tt := range []struct {
		name   string
		method string
		body   string
	}{
		{
			name:   "test GET",
			method: "GET",
		},
		{
			name:   "test POST",
			method: "POST",
			body:   "hello POST",
		},
		{
			name:   "test PATCH",
			method: "PATCH",
			body:   "hello PATCH",
		},
		{
			name:   "test PUT",
			method: "PUT",
			body:   "hello PUT",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			i := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if i == 0 {
					i++
					w.WriteHeader(http.StatusBadGateway)
					return
				}

				got, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("got no data")
				}
				s := string(got)
				if tt.body != s {
					t.Fatalf("Failed to get the right data want: %q, got: %q", tt.body, s)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			noleak.Check(t)

			fr := make(filters.Registry)
			retry := NewRetry()
			fr.Register(retry)
			r := &eskip.Route{
				Filters: []*eskip.Filter{
					{Name: retry.Name()},
				},
				Backend: backend.URL,
			}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()

			buf := bytes.NewBufferString(tt.body)
			req, err := http.NewRequest(tt.method, proxy.URL, buf)
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to execute retry: %v", err)
			}

			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status code: %s", rsp.Status)
			}
			rsp.Body.Close()
		})
	}
}
