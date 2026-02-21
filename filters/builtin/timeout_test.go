package builtin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestTimeoutCreateFilterEdgeCases(t *testing.T) {
	ti := &timeout{}
	if ti.Name() != "unknownFilter" {
		t.Errorf("wrong name: %s", ti.Name())
	}

	ti.typ = readTimeout
	_, err := ti.CreateFilter([]any{3 * time.Microsecond})
	if err != nil {
		t.Errorf("Failed to create filter: %v", err)
	}

	_, err = ti.CreateFilter([]any{"3.1"})
	if err == nil {
		t.Error("Failed to get filter error on wrong input string")
	}

	_, err = ti.CreateFilter([]any{3.5})
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("Failed to get filter error on wrong input: %v", err)
	}
	_, err = ti.CreateFilter([]any{5, "5"})
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("Failed to get filter error on too many args: %v", err)
	}
	_, err = ti.CreateFilter([]any{})
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("Failed to get filter error on too few args: %v", err)
	}

}

func TestBackendTimeout(t *testing.T) {
	bt := NewBackendTimeout()
	if bt.Name() != filters.BackendTimeoutName {
		t.Error("wrong name")
	}

	f, err := bt.CreateFilter([]any{"2s"})
	if err != nil {
		t.Error("wrong id")
	}

	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: make(map[string]any)}
	f.Request(c)

	if c.FStateBag[filters.BackendTimeout] != 2*time.Second {
		t.Error("wrong timeout")
	}

	// second filter overwrites
	f, _ = bt.CreateFilter([]any{"5s"})
	f.Request(c)

	if c.FStateBag[filters.BackendTimeout] != 5*time.Second {
		t.Error("overwrite expected")
	}
}

func TestTimeoutsFilterBackendTimeout(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     string
		workTime time.Duration
		want     int
		wantErr  bool
	}{
		{
			name:     "BackendTimeout bigger than backend time should return 200",
			args:     "1s",
			workTime: 100 * time.Millisecond,
			want:     http.StatusOK,
			wantErr:  false,
		}, {
			name:     "BackendTimeout smaller than backend time should timeout",
			args:     "10ms",
			workTime: 100 * time.Millisecond,
			want:     http.StatusGatewayTimeout,
			wantErr:  false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				time.Sleep(tt.workTime)

				defer r.Body.Close()

				_, err := io.Copy(io.Discard, r.Body)
				if err != nil {
					t.Logf("body read timeout: %v", err)
					w.WriteHeader(499)
					w.Write([]byte("client timeout: " + err.Error()))
					return
				}

				ctx := r.Context()
				select {
				case <-ctx.Done():
					if err := ctx.Err(); err != nil {
						t.Logf("backend handler observes error form context: %v", err)
						w.WriteHeader(498) // ???
						w.Write([]byte("context error: " + err.Error()))
						return
					}
				default:
					t.Log("default")
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			fr := make(filters.Registry)
			filter := NewBackendTimeout().(*timeout)

			fr.Register(filter)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: filter.Name(), Args: []any{tt.args}}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}

			client := net.NewClient(net.Options{})
			defer client.Close()

			var req *http.Request
			req, err = http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := client.Do(req)

			if err != nil {
				t.Fatal(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}
			_, err = io.Copy(io.Discard, rsp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)

			}
		})
	}

}

func TestTimeoutsFilterReadTimeout(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     string
		workTime time.Duration
		want     int
		wantErr  bool
	}{
		{
			name:     "ReadTimeout bigger than reading time should return 200",
			args:     "1s",
			workTime: 10 * time.Millisecond,
			want:     http.StatusOK,
			wantErr:  false,
		}, {
			name:     "ReadTimeout smaller than reading time should timeout",
			args:     "15ms",
			workTime: 5 * time.Millisecond,
			want:     499,
			wantErr:  false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()

				_, err := io.Copy(io.Discard, r.Body)
				if err == io.ErrUnexpectedEOF {
					t.Logf("body read timeout: %v", err)
					w.WriteHeader(495)
					w.Write([]byte("does not matter: " + err.Error()))
					return
				}

				ctx := r.Context()
				select {
				case <-ctx.Done():
					if err := ctx.Err(); err != nil {
						t.Fatalf("backend handler observes error form context: %v", err)
					}
				default:
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			fr := make(filters.Registry)
			filter := NewReadTimeout().(*timeout)

			fr.Register(filter)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: filter.Name(), Args: []any{tt.args}}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}

			client := net.NewClient(net.Options{})
			defer client.Close()

			var req *http.Request
			dat := bytes.NewBufferString("abcdefghijklmn")
			req, err = http.NewRequest("POST", reqURL.String(), &slowReader{
				data: dat,
				d:    tt.workTime,
			})
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := client.Do(req)

			if err != nil {
				t.Fatal(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}
			_, err = io.Copy(io.Discard, rsp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)

			}
		})
	}

}

func TestTimeoutsFilterWriteTimeout(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     string
		workTime time.Duration
		want     int
		wantErr  bool
	}{
		{
			name:     "WriteTimeout bigger than writing time should return 200",
			args:     "1s",
			workTime: 10 * time.Millisecond,
			want:     http.StatusOK,
			wantErr:  false,
		}, {
			name:     "WriteTimeout smaller than writing time should timeout",
			args:     "25ms",
			workTime: 35 * time.Millisecond,
			want:     http.StatusOK, // because response headers already sent
			wantErr:  true,          // because write timeout
		}} {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				_, err := io.Copy(io.Discard, r.Body)
				if err != nil {
					t.Logf("Failed to copy body: %v", err)
				}

				w.WriteHeader(http.StatusOK)

				now := time.Now()
				for range 5 {
					n, err := w.Write([]byte(strings.Repeat("a", 8192)))
					if err != nil {
						t.Logf("Failed to write: %v", err)
						break
					}
					t.Logf("Wrote %d bytes, %s", n, time.Since(now))
					time.Sleep(tt.workTime)
				}

			}))
			defer backend.Close()

			fr := make(filters.Registry)
			filter := NewWriteTimeout().(*timeout)

			fr.Register(filter)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: filter.Name(), Args: []any{tt.args}}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}

			client := net.NewClient(net.Options{})
			defer client.Close()

			var req *http.Request
			req, err = http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			rsp, err := client.Do(req)

			// test write timing out
			if tt.wantErr {
				n, err := io.Copy(io.Discard, rsp.Body)
				t.Logf("Copied %d bytes from response body", n)
				if err != nil {
					if err == io.ErrUnexpectedEOF {
						t.Log("Expected error from io due to write deadline on the server side")
					} else {
						t.Fatalf("Failed to read response body: %v", err)
					}
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != tt.want {
				t.Fatalf("Failed to get %d, got %d", tt.want, rsp.StatusCode)
			}
			_, err = io.Copy(io.Discard, rsp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
		})
	}

}

type slowReader struct {
	data *bytes.Buffer
	d    time.Duration
}

func (sr *slowReader) Read(b []byte) (int, error) {
	r := io.LimitReader(sr.data, 2)
	n, err := r.Read(b)
	time.Sleep(sr.d)
	return n, err
}
