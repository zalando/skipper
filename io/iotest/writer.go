package iotest

import (
	"io"
	"net/http"
	"testing/iotest"
	"time"
)

type FlushedResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	Unwrap() http.ResponseWriter
}

// SlowResponseWriter is a FlushedResponseWriter to test write timeout
// specific test scenarios.
type SlowResponseWriter struct {
	rw http.ResponseWriter
	w  FlushedWriter
}

var _ FlushedResponseWriter = &SlowResponseWriter{}

// NewSlowResponseWriter creates a new SlowResponseWriter wrapping the
// given http.ResponseWriter with a delay after each byte.
func NewSlowResponseWriter(rw http.ResponseWriter, d time.Duration) *SlowResponseWriter {
	return &SlowResponseWriter{
		rw: rw,
		w:  NewSlowWriter(rw, d),
	}
}

func (sw *SlowResponseWriter) Header() http.Header {
	return sw.rw.Header()
}

func (sw *SlowResponseWriter) WriteHeader(i int) {
	sw.rw.WriteHeader(i)
}

func (sw *SlowResponseWriter) Flush() {
	sw.w.(http.Flusher).Flush()
}

func (sw *SlowResponseWriter) Unwrap() http.ResponseWriter {
	return sw.rw
}

func (sw *SlowResponseWriter) Write(p []byte) (n int, err error) {
	return sw.w.Write(p)
}

type FlushedWriter interface {
	io.Writer
	http.Flusher
}

var (
	_ FlushedWriter = &SlowWriter{}
	_ FlushedWriter = &flushedWriter{}
)

type SlowWriter struct {
	w io.Writer
	d time.Duration
}

func NewSlowWriter(w io.Writer, d time.Duration) *SlowWriter {
	return &SlowWriter{
		w: w,
		d: d,
	}
}

// Write implements the io.Writer interface.  It writes one byte at a
// time to the underlying writer, sleeps for the specified delay,
// reading from buffer 'p'. Each write will be followed by a flush.
func (sw *SlowWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for i := range len(p) {
		oneByte := make([]byte, 1)
		oneByte[0] = p[i]
		bytesWrite, err := sw.w.Write(oneByte)

		if err != nil {
			if n > 0 && err == io.EOF {
				return n, nil
			}
			return n, err
		}
		sw.Flush()

		// single byte write
		if bytesWrite == 1 {
			n++
			time.Sleep(sw.d)
		} else {
			return n, nil
		}
	}

	return n, nil
}

func (sw *SlowWriter) Flush() {
	sw.w.(http.Flusher).Flush()
}

type flushedWriter struct {
	w io.Writer
}

func newFlushedWriter(w io.Writer) *flushedWriter {
	return &flushedWriter{
		w: w,
	}
}

func (fw *flushedWriter) Write(p []byte) (int, error) {
	return fw.w.Write(p)
}

func (fw *flushedWriter) Flush() {}

type TimeoutWriter struct {
	w       io.Writer
	attempt int
	count   int
}

func NewTimeoutWriter(w io.Writer, attempt int) *TimeoutWriter {
	return &TimeoutWriter{
		w:       w,
		attempt: attempt,
	}
}

func (tw *TimeoutWriter) Write(p []byte) (int, error) {
	println("tw.Write")
	if tw.count >= tw.attempt {
		return 0, iotest.ErrTimeout
	}
	tw.count++
	return tw.w.Write(p)
}
