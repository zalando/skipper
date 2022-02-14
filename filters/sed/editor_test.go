package sed

import (
	"bytes"
	"io"
	"regexp"
	"testing"
)

// Here testing certain edge cases that are hard reproduce
// when running the editor as a filter in the proxy.

type nonBlockingReader struct {
	initialContent []byte
}

type infiniteReader struct {
	content []byte
}

type errorReader string

func (r *nonBlockingReader) Read(p []byte) (int, error) {
	n := copy(p, r.initialContent)
	r.initialContent = r.initialContent[n:]
	return n, nil
}

func (r *infiniteReader) Read(p []byte) (int, error) {
	l := len(p)
	for {
		if len(p) == 0 {
			return l, nil
		}

		n := copy(p, r.content)
		p = p[n:]
		r.content = append(r.content[n:], r.content[:n]...)
	}
}

func (r errorReader) Read([]byte) (int, error) { return 0, r }
func (r errorReader) Error() string            { return string(r) }

func TestEditorNonBlockingSource(t *testing.T) {
	r := &nonBlockingReader{initialContent: []byte("fox")}
	e := newEditor(r, regexp.MustCompile("fo"), []byte("ba"), nil, 0, maxBufferBestEffort)

	// hook the read buffer:
	e.readBuffer = make([]byte, 2)

	p := make([]byte, 4)
	n, err := e.Read(p)
	if err != nil {
		t.Fatal(err)
	}

	if n != 2 || string(p[:2]) != "ba" {
		t.Fatal("failed to read the data", n, string(p[:2]))
	}

	n, err = e.Read(p)
	if err != nil {
		t.Fatal(err)
	}

	if n != 0 {
		t.Fatal("failed to stop reading")
	}
}

func TestEditorMaxBuffer(t *testing.T) {
	t.Run("buffer matches", func(t *testing.T) {
		r := bytes.NewBufferString("foobarbaz")
		e := newEditor(r, regexp.MustCompile("[a-z]+"), []byte("x"), nil, 3, maxBufferBestEffort)

		// hook the read buffer:
		e.readBuffer = make([]byte, 2)

		p, err := io.ReadAll(e)
		if err != nil {
			t.Fatal(err)
		}

		if string(p) != "xx" {
			t.Error("failed to edit content", string(p))
		}
	})

	t.Run("buffer does not match", func(t *testing.T) {
		r := bytes.NewBufferString("foobarbaz")
		e := newEditor(r, regexp.MustCompile("[a-z]+x"), []byte("x"), nil, 3, maxBufferBestEffort)

		// hook the read buffer:
		e.readBuffer = make([]byte, 2)

		p, err := io.ReadAll(e)
		if err != nil {
			t.Fatal(err)
		}

		if string(p) != "foobarbaz" {
			t.Error("failed to edit content", string(p))
		}
	})

	t.Run("match over boundary", func(t *testing.T) {
		r := bytes.NewBufferString("foox")
		e := newEditor(r, regexp.MustCompile("foox"), []byte("barx"), nil, 1, maxBufferBestEffort)

		// hook the read buffer:
		e.readBuffer = make([]byte, 2)

		p := make([]byte, 4)
		for i := 0; i < 2; i++ {
			n, err := e.Read(p[i*2 : i*2+2])
			if err != nil {
				t.Fatal(err)
			}

			if n != 2 {
				t.Fatal("failed to read enough")
			}
		}

		if string(p) != "foox" {
			t.Error("failed to edit content", string(p))
		}
	})
}

func TestEditorIncreasingReadSize(t *testing.T) {
	r := &infiniteReader{content: []byte("foobarbaz")}
	e := newEditor(r, regexp.MustCompile("[a-z]x"), []byte("bar"), nil, 128, maxBufferBestEffort)

	// hook the read buffer:
	e.readBuffer = make([]byte, 2)

	b, err := io.ReadAll(io.LimitReader(e, 27))
	if err != nil {
		t.Fatal(err)
	}

	if string(b) != "foobarbazfoobarbazfoobarbaz" {
		t.Error("failed to edit content", string(b))
	}
}

func TestEditorInfiniteInput(t *testing.T) {
	t.Run("prefixable", func(t *testing.T) {
		r := &infiniteReader{content: []byte("foobarbaz")}
		e := newEditor(r, regexp.MustCompile("foo"), []byte("bar"), nil, 0, maxBufferBestEffort)

		// hook the read buffer:
		e.readBuffer = make([]byte, 2)

		b, err := io.ReadAll(io.LimitReader(e, 27))
		if err != nil {
			t.Fatal(err)
		}

		if string(b) != "barbarbazbarbarbazbarbarbaz" {
			t.Error("failed to edit content", string(b))
		}
	})

	t.Run("non-prefixable", func(t *testing.T) {
		r := &infiniteReader{content: []byte("foobarbaz")}
		e := newEditor(r, regexp.MustCompile("[a-z]oo"), []byte("bar"), nil, 0, maxBufferBestEffort)

		// hook the read buffer:
		e.readBuffer = make([]byte, 2)

		b, err := io.ReadAll(io.LimitReader(e, 27))
		if err != nil {
			t.Fatal(err)
		}

		if string(b) != "barbarbazbarbarbazbarbarbaz" {
			t.Error("failed to edit content", string(b))
		}
	})
}

func TestEditorTransparentError(t *testing.T) {
	r := errorReader("test error")
	e := newEditor(r, regexp.MustCompile(""), nil, nil, 0, maxBufferBestEffort)
	if _, err := e.Read(make([]byte, 3)); err != r {
		t.Error("failed to transparently pass through the error")
	}
}
