package block

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
)

type nonBlockingReader struct {
	initialContent []byte
}

func (r *nonBlockingReader) Read(p []byte) (int, error) {
	n := copy(p, r.initialContent)
	r.initialContent = r.initialContent[n:]
	return n, nil
}

func (r *nonBlockingReader) Close() error {
	return nil
}

func TestMatcher(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		block   []byte
		err     error
	}{
		{
			name:    "empty string",
			content: "",
			block:   []byte(".class"),
			err:     nil,
		},
		{
			name:    "small string",
			content: ".class",
			block:   []byte(".class"),
			err:     net.ErrBlocked,
		},
		{
			name:    "small string without match",
			content: "foxi",
			block:   []byte(".class"),
			err:     nil,
		},
		{
			name:    "small string with match",
			content: "fox.class.foo.blah",
			block:   []byte(".class"),
			err:     net.ErrBlocked,
		},
		{
			name:    "hex string 0x00 without match",
			content: "fox.c0.foo.blah",
			block:   []byte("\x00"),
		},
		{
			name:    "hex string 0x00 with match",
			content: "fox.c\x00.foo.blah",
			block:   []byte("\x00"),
			err:     net.ErrBlocked,
		},
		{
			name:    "hex string with uppercase match content string with lowercase",
			content: "fox.c\x0A.foo.blah",
			block:   []byte("\x0a"),
			err:     net.ErrBlocked,
		},
		{
			name:    "hex string 0x00 0x0a with match",
			content: "fox.c\x00\x0a.foo.blah",
			block:   []byte{0, 10},
			err:     net.ErrBlocked,
		},
		{
			name:    "long string",
			content: strings.Repeat("A", 8192),
			block:   []byte(".class"),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			r := &nonBlockingReader{initialContent: []byte(tt.content)}
			toblockList := []toBlockKeys{{Str: tt.block}}

			req, err := http.NewRequest("POST", "http://test.example", r)
			if err != nil {
				t.Fatalf("Failed to create request with body: %v", err)
			}

			bmb := net.WrapBody(req.Context(), blockMatcher(metrics.Default, toblockList), req.Body)

			p := make([]byte, len(r.initialContent))
			n, err := bmb.Read(p)
			if err != tt.err {
				t.Fatalf("Failed to get expected err %v, got: %v", tt.err, err)
			}
			if err != nil {
				if err == net.ErrBlocked {
					t.Logf("Stop! Request has some blocked content!")
				} else {
					t.Errorf("Failed to read: %v", err)
				}
			} else if n != len(tt.content) {
				t.Errorf("Failed to read content length %d, got %d", len(tt.content), n)
			}
		})
	}
}

func TestBlockCreateFilterErrors(t *testing.T) {
	spec := NewBlock(1024)

	t.Run("empty args", func(t *testing.T) {
		args := []interface{}{}
		if _, err := spec.CreateFilter(args); err == nil {
			t.Error("CreateFilter with empty args should fail")
		}
	})

	t.Run("non string args", func(t *testing.T) {
		args := []interface{}{3}
		if _, err := spec.CreateFilter(args); err == nil {
			t.Error("CreateFilter with non string args should fail")
		}
	})
}

func TestBlock(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	spec := NewBlock(1024)
	fr := make(filters.Registry)
	fr.Register(spec)

	t.Run("block request", func(t *testing.T) {
		args := []interface{}{"foo"}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello foo world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 400 {
			t.Errorf("Not Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("block request chain first blocks", func(t *testing.T) {
		args := []interface{}{"foo"}
		args2 := []interface{}{"bar"}
		r := &eskip.Route{
			Filters: []*eskip.Filter{
				{Name: spec.Name(), Args: args},
				{Name: spec.Name(), Args: args2},
			}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello foo world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 400 {
			t.Errorf("Not Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("block request chain second blocks", func(t *testing.T) {
		args := []interface{}{"bar"}
		args2 := []interface{}{"foo"}
		r := &eskip.Route{
			Filters: []*eskip.Filter{
				{Name: spec.Name(), Args: args},
				{Name: spec.Name(), Args: args2},
			}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello foo world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 400 {
			t.Errorf("Not Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("pass request", func(t *testing.T) {
		args := []interface{}{"foo"}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("pass request with filter chain and check content", func(t *testing.T) {
		content := "hello world"

		be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte("Failed to read body"))
				return
			}
			if s := string(res); s != content {
				t.Logf("backend received: %q", s)
				w.WriteHeader(400)
				w.Write([]byte("wrong body"))
				return
			}
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		}))
		defer be.Close()

		args := []interface{}{"foo"}
		args2 := []interface{}{"bar"}
		r := &eskip.Route{Filters: []*eskip.Filter{
			{Name: spec.Name(), Args: args},
			{Name: spec.Name(), Args: args2},
		}, Backend: be.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString(content)
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := proxy.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		result, _ := io.ReadAll(rsp.Body)
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d: %s", rsp.StatusCode, string(result))
		}
	})

	t.Run("pass request on empty body", func(t *testing.T) {
		args := []interface{}{"foo"}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d", rsp.StatusCode)
		}
	})
	t.Run("pass request on empty body with filter chain", func(t *testing.T) {
		args := []interface{}{"foo"}
		args2 := []interface{}{"bar"}
		r := &eskip.Route{Filters: []*eskip.Filter{
			{Name: spec.Name(), Args: args},
			{Name: spec.Name(), Args: args2},
		}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d", rsp.StatusCode)
		}
	})
}

func TestBlockHex(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	spec := NewBlockHex(1024)
	fr := make(filters.Registry)
	fr.Register(spec)

	t.Run("block request", func(t *testing.T) {
		args := []interface{}{`000a`}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello \x00\x0afoo world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 400 {
			t.Errorf("Not Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("block request binary data in request", func(t *testing.T) {
		args := []interface{}{`000a`}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBuffer([]byte{65, 65, 31, 0, 10, 102, 111, 111, 31})
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 400 {
			t.Errorf("Not Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("pass request", func(t *testing.T) {
		args := []interface{}{`000a`}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBufferString("hello \x00a\x0a world")
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d", rsp.StatusCode)
		}
	})

	t.Run("pass request binary data in request", func(t *testing.T) {
		args := []interface{}{`000a`}
		r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}
		proxy := proxytest.New(fr, r)
		defer proxy.Close()
		reqURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
		}

		buf := bytes.NewBuffer([]byte{65, 65, 31, 0, 11, 102, 111, 111, 31})
		req, err := http.NewRequest("POST", reqURL.String(), buf)
		if err != nil {
			t.Fatal(err)
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != 200 {
			t.Errorf("Blocked response status code %d", rsp.StatusCode)
		}
	})
}
