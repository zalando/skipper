package log

import (
	"bytes"
	"io"
)

type teeBody struct {
	body      io.ReadCloser
	buffer    *bytes.Buffer
	teeReader io.Reader
	maxTee    int
}

func newTeeBody(rc io.ReadCloser, maxTee int) io.ReadCloser {
	b := bytes.NewBuffer(nil)
	tb := &teeBody{
		body:   rc,
		buffer: b,
		maxTee: maxTee}
	tb.teeReader = io.TeeReader(rc, tb)
	return tb
}

func (tb *teeBody) Read(b []byte) (int, error) { return tb.teeReader.Read(b) }
func (tb *teeBody) Close() error               { return tb.body.Close() }

func (tb *teeBody) Write(b []byte) (int, error) {
	if tb.maxTee < 0 {
		return tb.buffer.Write(b)
	}

	wl := len(b)
	if wl >= tb.maxTee {
		wl = tb.maxTee
	}

	n, err := tb.buffer.Write(b[:wl])
	if err != nil {
		return n, err
	}

	tb.maxTee -= n

	// lie to avoid short write
	return len(b), nil
}
