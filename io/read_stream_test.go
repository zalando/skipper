package io

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

type toBlockKeys struct{ Str []byte }

func blockMatcher(matches []toBlockKeys) func(b []byte) (int, error) {
	return func(b []byte) (int, error) {
		var consumed int
		for _, s := range matches {
			if bytes.Contains(b, s.Str) {
				return 0, ErrBlocked
			}
		}
		consumed += len(b)
		return consumed, nil
	}
}

func TestHttpBodyReadOnly(t *testing.T) {
	sent := "hell0 foo bar"

	okBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 0, 1024)
		buf := bytes.NewBuffer(b)
		n, err := io.Copy(buf, r.Body)
		if err != nil {
			t.Fatalf("Failed to read body on backend receiver: %v", err)
		}

		t.Logf("read(%d): %s", n, buf)
		if got := buf.String(); got != sent {
			t.Fatalf("Failed to get request body in okbackend. want: %q, got: %q", sent, got)
		}
		w.WriteHeader(200)
		// w.Write([]byte("OK"))
		w.Write(b[:n])
	}))
	defer okBackend.Close()

	blockedBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1024)
		buf := bytes.NewBuffer(b)
		_, err := io.Copy(buf, r.Body)

		// body started to stream but was cut by sender
		if err != io.ErrUnexpectedEOF {
			t.Logf("expected 'io.ErrUnexpectedEOF' got: %v", err)
		}

		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer blockedBackend.Close()

	t.Run("single block matcher without match", func(t *testing.T) {
		var b mybuf
		b.buf = bytes.NewBufferString(sent)

		body := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("no match")}}), b)
		defer body.Close()
		rsp, err := (&http.Client{}).Post(okBackend.URL, "text/plain", body)
		if err != nil {
			t.Fatalf("Failed to do POST request: %v", err)
		}

		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get the expected status code 200, got: %d", rsp.StatusCode)
		}
		var buf bytes.Buffer
		io.Copy(&buf, rsp.Body)
		rsp.Body.Close()
		if got := buf.String(); got != sent {
			t.Fatalf("Failed to get %q, got %q", sent, got)
		}
	})

	t.Run("double block matcher without match", func(t *testing.T) {
		var b mybuf
		b.buf = bytes.NewBufferString(sent)

		bod := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("no-match")}}), b)
		defer bod.Close()
		body := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("no match")}}), bod)
		defer body.Close()
		rsp, err := (&http.Client{}).Post(okBackend.URL, "text/plain", body)
		if err != nil {
			t.Fatalf("Failed to POST request: %v", err)
		}

		if rsp.StatusCode != http.StatusOK {
			t.Fatalf("Failed to get 200 status code, got: %v", rsp.StatusCode)
		}
		var buf bytes.Buffer
		io.Copy(&buf, rsp.Body)
		rsp.Body.Close()
		if got := buf.String(); got != sent {
			t.Fatalf("Failed to get %q, got %q", sent, got)
		}
	})

	t.Run("single block matcher with match", func(t *testing.T) {

		var b mybuf
		b.buf = bytes.NewBufferString("hell0 foo bar")

		body := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("foo")}}), b)
		defer body.Close()
		rsp, err := (&http.Client{}).Post(blockedBackend.URL, "text/plain", body)
		if !errors.Is(err, ErrBlocked) {
			if rsp != nil {
				t.Errorf("rsp should be nil, status code: %d", rsp.StatusCode)
			}
			t.Fatalf("Expected POST request to be blocked, got err: %v", err)
		}
	})

	t.Run("double block matcher with first match", func(t *testing.T) {
		var b mybuf
		b.buf = bytes.NewBufferString("hell0 foo bar")

		body := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("foo")}}), b)
		body = InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("no match")}}), body)
		defer body.Close()
		rsp, err := (&http.Client{}).Post(blockedBackend.URL, "text/plain", body)

		if !errors.Is(err, ErrBlocked) {
			if rsp != nil {
				t.Errorf("rsp should be nil, status code: %d", rsp.StatusCode)
			}
			t.Fatalf("Expected POST request to be blocked, got err: %v", err)
		}
	})

	t.Run("double block matcher with second match", func(t *testing.T) {
		var b mybuf
		b.buf = bytes.NewBufferString("hell0 foo bar")

		body := InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("no match")}}), b)
		body = InspectReader(context.Background(), BufferOptions{}, blockMatcher([]toBlockKeys{{Str: []byte("bar")}}), body)
		defer body.Close()
		rsp, err := (&http.Client{}).Post(blockedBackend.URL, "text/plain", body)

		if !errors.Is(err, ErrBlocked) {
			if rsp != nil {
				t.Errorf("rsp should be nil, status code: %d", rsp.StatusCode)
			}
			t.Fatalf("Expected POST request to be blocked, got err: %v", err)
		}
	})

}

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

func (hr *hookReader) Close() error {
	return nil
}

type slowBlockingReader struct {
	initialContent []byte
}

func (r *slowBlockingReader) Read(p []byte) (int, error) {
	time.Sleep(250 * time.Millisecond)
	n := copy(p, r.initialContent)
	r.initialContent = r.initialContent[n:]
	return n, nil
}

func (r *slowBlockingReader) Close() error {
	return nil
}

type hookReader struct {
	initialContent []byte
	nbytes         int
	hook           func()
	counter        int
}

func (hr *hookReader) Read(p []byte) (int, error) {
	println("Read()", len(p))
	if len(hr.initialContent) < hr.nbytes || len(p) < hr.nbytes {
		return 0, nil
	}
	n := copy(p, hr.initialContent[:hr.nbytes])
	hr.initialContent = hr.initialContent[n:]
	hr.hook()
	if hr.counter > 0 {
		hr.counter--
		return n, syscall.EAGAIN
	}
	return n, nil
}

func TestMatcherFuncError(t *testing.T) {
	t.Run("test canceled while matcher is running", func(t *testing.T) {
		rc := &hookReader{
			initialContent: []byte("0123456789abcdef"),
			hook: func() {
				time.Sleep(11 * time.Millisecond)
			},
			nbytes: 4,
		}

		ctx, done := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer done()

		f := func(p []byte) (int, error) {
			return len(p), nil
		}

		m := newMatcher(ctx, rc, f, 1024, MaxBufferBestEffort)

		p := make([]byte, 8)
		_, err := m.Read(p)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Failed to read: %v", err)
		}
	})

	t.Run("test read error", func(t *testing.T) {
		rc := &hookReader{
			initialContent: []byte("0123456789abcdef"),
			hook: func() {
				time.Sleep(5 * time.Millisecond)
			},
			nbytes: 4,
		}
		errTest := fmt.Errorf("we test an error")

		f := func(p []byte) (int, error) {
			if len(p) == 8 {
				return 0, errTest
			}
			return len(p), nil
		}

		m := newMatcher(context.Background(), rc, f, 1024, MaxBufferBestEffort)

		p := make([]byte, 8)
		_, err := m.Read(p)

		if !errors.Is(err, errTest) {
			t.Fatalf("Failed to read: %v", err)
		}
	})

	t.Run("test pending read func error", func(t *testing.T) {
		rc := &hookReader{
			initialContent: []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
			hook:           func() {},
			nbytes:         4,
		}
		errTest := fmt.Errorf("we test an error")

		f := func(p []byte) (int, error) {
			switch len(p) {
			case 12:
				return 0, errTest
			}
			return len(p), nil
		}

		m := newMatcher(context.Background(), rc, f, 8, MaxBufferBestEffort)

		p := make([]byte, 8)
		_, err := m.Read(p)

		if !errors.Is(err, errTest) {
			t.Fatalf("Failed to read: %v", err)
		}
	})
}

// TODO(sszuecs): test all error cases for matcher, the following we had for blockContent() filter
func TestMatcherErrorCases(t *testing.T) {
	toblockList := []toBlockKeys{{Str: []byte(".class")}}
	t.Run("maxBufferAbort", func(t *testing.T) {
		r := &nonBlockingReader{initialContent: []byte("fppppppppp .class")}
		bmb := newMatcher(context.Background(), r, blockMatcher(toblockList), 5, MaxBufferAbort)
		p := make([]byte, len(r.initialContent))
		_, err := bmb.Read(p)
		if err != ErrMatcherBufferFull {
			t.Errorf("Failed to get expected error %v, got: %v", ErrMatcherBufferFull, err)
		}
	})

	t.Run("maxBuffer", func(t *testing.T) {
		r := &nonBlockingReader{initialContent: []byte("fppppppppp .class")}
		bmb := newMatcher(context.Background(), r, blockMatcher(toblockList), 5, MaxBufferBestEffort)
		p := make([]byte, len(r.initialContent))
		_, err := bmb.Read(p)
		if err != nil {
			t.Errorf("Failed to read: %v", err)
		}
	})

	t.Run("cancel read", func(t *testing.T) {
		r := &slowBlockingReader{initialContent: []byte("fppppppppp .class")}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Millisecond))
		defer cancel()
		bmb := newMatcher(ctx, r, blockMatcher(toblockList), 5, MaxBufferBestEffort)
		p := make([]byte, len(r.initialContent))
		_, err := bmb.Read(p)
		if err == nil {
			t.Errorf("Failed to cancel read: %v", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Failed to get deadline exceeded, got: %T", err)
		}
	})

	t.Run("maxBuffer read on closed reader", func(t *testing.T) {
		pipeR, pipeW := io.Pipe()
		initialContent := []byte("fppppppppp")
		go pipeW.Write(initialContent)
		bmb := newMatcher(context.Background(), pipeR, blockMatcher(toblockList), 5, MaxBufferBestEffort)
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
		bmb := newMatcher(context.Background(), pipeR, blockMatcher(toblockList), 5, MaxBufferBestEffort)
		p := make([]byte, len(initialContent)+10)
		pipeR.Close()
		bmb.Close()
		_, err := bmb.Read(p)
		if err == nil || err.Error() != "reader closed" {
			t.Errorf("Failed to get correct read error: %v", err)
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
			toblockList := []toBlockKeys{{Str: tt.tomatch}}
			bmb := newMatcher(context.Background(), r.Body, blockMatcher(toblockList), 2097152, MaxBufferBestEffort)
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
