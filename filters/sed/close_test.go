package sed

import (
	"regexp"
	"strings"
	"testing"
)

type testReadCloser struct {
	closed bool
}

func (testReadCloser) Read(p []byte) (int, error) {
	copy(p, []byte{1, 2, 3})
	return 3, nil
}

func (r *testReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestClose(t *testing.T) {
	t.Run("close", func(t *testing.T) {
		r := &testReadCloser{}
		e := newEditor(r, regexp.MustCompile("foo"), []byte("bar"), nil, 0, maxBufferBestEffort)
		if _, err := e.Read(make([]byte, 6)); err != nil {
			t.Fatal(err)
		}

		if err := e.Close(); err != nil {
			t.Fatal(err)
		}

		if !r.closed {
			t.Error("failed to close the underlying reader")
		}
	})

	t.Run("close when there is still something ready", func(t *testing.T) {
		r := &testReadCloser{}
		e := newEditor(r, regexp.MustCompile("foo"), []byte("bar"), nil, 0, maxBufferBestEffort)
		if _, err := e.Read(make([]byte, 2)); err != nil {
			t.Fatal(err)
		}

		if err := e.Close(); err != nil {
			t.Fatal(err)
		}

		if !r.closed {
			t.Fatal("failed to close the underlying reader")
		}

		if n, err := e.Read(make([]byte, 2)); err != ErrClosed {
			t.Error("failed to fail with the right error", n, err)
		}
	})

	t.Run("not a closer", func(t *testing.T) {
		r := strings.NewReader("foobarbaz")
		e := newEditor(r, regexp.MustCompile("foo"), []byte("bar"), nil, 0, maxBufferBestEffort)
		if _, err := e.Read(make([]byte, 6)); err != nil {
			t.Fatal(err)
		}

		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
}
