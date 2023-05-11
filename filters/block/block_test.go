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
	"github.com/zalando/skipper/proxy"
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
			err:     nil,
		},
		{
			name:    "small string",
			content: ".class",
			err:     proxy.ErrBlocked,
		},
		{
			name:    "small string without match",
			content: "foxi",
			err:     nil,
		},
		{
			name:    "small string with match",
			content: "fox.class.foo.blah",
			err:     proxy.ErrBlocked,
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
			err:     proxy.ErrBlocked,
		},
		{
			name:    "hex string with uppercase match content string with lowercase",
			content: "fox.c\x0A.foo.blah",
			block:   []byte("\x0a"),
			err:     proxy.ErrBlocked,
		},
		{
			name:    "hex string 0x00 0x0a with match",
			content: "fox.c\x00\x0a.foo.blah",
			block:   []byte{0, 10},
			err:     proxy.ErrBlocked,
		},
		{
			name:    "long string",
			content: strings.Repeat("A", 8192),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			block := []byte(".class")
			if len(tt.block) != 0 {
				block = tt.block
			}
			r := &nonBlockingReader{initialContent: []byte(tt.content)}
			toblockList := []toblockKeys{{str: block}}

			bmb := newMatcher(r, toblockList, 2097152, maxBufferBestEffort)

			t.Logf("Content: %s", r.initialContent)
			p := make([]byte, len(r.initialContent))
			n, err := bmb.Read(p)
			if err != tt.err {
				t.Fatalf("Failed to get expected err %v, got: %v", tt.err, err)
			}
			if err != nil {
				if err == proxy.ErrBlocked {
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

func TestMatcherErrorCases(t *testing.T) {
	toblockList := []toblockKeys{{str: []byte(".class")}}
	t.Run("maxBufferAbort", func(t *testing.T) {
		r := &nonBlockingReader{initialContent: []byte("fppppppppp .class")}
		bmb := newMatcher(r, toblockList, 5, maxBufferAbort)
		p := make([]byte, len(r.initialContent))
		_, err := bmb.Read(p)
		if err != ErrMatcherBufferFull {
			t.Errorf("Failed to get expected error %v, got: %v", ErrMatcherBufferFull, err)
		}
	})

	t.Run("maxBuffer", func(t *testing.T) {
		r := &nonBlockingReader{initialContent: []byte("fppppppppp .class")}
		bmb := newMatcher(r, toblockList, 5, maxBufferBestEffort)
		p := make([]byte, len(r.initialContent))
		_, err := bmb.Read(p)
		if err != nil {
			t.Errorf("Failed to read: %v", err)
		}
	})

	t.Run("maxBuffer read on closed reader", func(t *testing.T) {
		pipeR, pipeW := io.Pipe()
		initialContent := []byte("fppppppppp")
		go pipeW.Write(initialContent)
		bmb := newMatcher(pipeR, toblockList, 5, maxBufferBestEffort)
		p := make([]byte, len(initialContent)+10)
		pipeR.Close()
		_, err := bmb.Read(p)
		if err == nil || err != io.ErrClosedPipe {
			t.Errorf("Failed to get correct read error: %v", err)
		}
	})

	t.Run("maxBuffer read on initial closed reader", func(t *testing.T) {
		pipeR, _ := io.Pipe()
		initialContent := []byte("fppppppppp")
		bmb := newMatcher(pipeR, toblockList, 5, maxBufferBestEffort)
		p := make([]byte, len(initialContent)+10)
		pipeR.Close()
		bmb.Close()
		_, err := bmb.Read(p)
		if err == nil || err.Error() != "reader closed" {
			t.Errorf("Failed to get correct read error: %v", err)
		}
	})
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
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	spec := NewBlock(1024)
	args := []interface{}{"foo"}
	fr := make(filters.Registry)
	fr.Register(spec)
	r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

	proxy := proxytest.New(fr, r)
	defer proxy.Close()
	reqURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
	}

	t.Run("block request", func(t *testing.T) {
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

	t.Run("pass request on empty body", func(t *testing.T) {
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
	args := []interface{}{`000a`}
	fr := make(filters.Registry)
	fr.Register(spec)
	r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

	proxy := proxytest.New(fr, r)
	defer proxy.Close()
	reqURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
	}

	t.Run("block request", func(t *testing.T) {
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

func BenchmarkBlock(b *testing.B) {

	fake := func(source string, len int) string {
		return strings.Repeat(source[:2], len) // partially matches target
	}

	fakematch := func(source string, len int) string {
		return strings.Repeat(source, len) // matches target
	}

	for _, tt := range []struct {
		name    string
		tomatch []byte
		bm      []byte
	}{
		{
			name:    "Small Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<20)), // Test with 1Mib
		},
		{
			name:    "Small Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<20)),
		},
		{
			name:    "Medium Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<24)), // Test with ~10Mib
		},
		{
			name:    "Medium Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<24)),
		},
		{
			name:    "Large Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<27)), // Test with ~100Mib
		},
		{
			name:    "Large Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<27)),
		}} {
		b.Run(tt.name, func(b *testing.B) {
			target := &nonBlockingReader{initialContent: tt.bm}
			r := &http.Request{
				Body: target,
			}
			toblockList := []toblockKeys{{str: tt.tomatch}}
			bmb := newMatcher(r.Body, toblockList, 2097152, maxBufferBestEffort)
			p := make([]byte, len(target.initialContent))
			b.Logf("Number of loops: %b", b.N)
			for n := 0; n < b.N; n++ {
				_, err := bmb.Read(p)
				if err != nil {
					return
				}
			}
		})
	}
}
