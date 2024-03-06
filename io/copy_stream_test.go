package io

import (
	"bytes"
	"io"
	"testing"
)

func TestCopyBodyStream(t *testing.T) {
	s := "content"
	bbuf := io.NopCloser(bytes.NewBufferString(s))
	cbs := NewCopyBodyStream(len(s), &bytes.Buffer{}, bbuf)

	buf := make([]byte, len(s))
	_, err := cbs.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if cbs.Len() != len(buf) {
		t.Fatalf("Failed to have the same buf buffer size want: %d, got: %d", cbs.Len(), len(buf))
	}

	got, err := io.ReadAll(cbs.GetBody())
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	if gotStr := string(got); s != gotStr {
		t.Fatalf("Failed to get the right content: %s != %s", s, gotStr)
	}

	if err = cbs.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCopyBodyStreamFailedRead(t *testing.T) {
	s := "content"
	bbuf := io.NopCloser(bytes.NewBufferString(s))

	failingBuf := &failingWriter{buf: &bytes.Buffer{}}

	cbs := NewCopyBodyStream(len(s), failingBuf, bbuf)

	buf := make([]byte, len(s))
	_, err := cbs.Read(buf)
	if err == nil {
		t.Fatal("Want to have failing buffer write inside Read()")
	}
}
