package io

import (
	"io"
)

type ReadWriterLen interface {
	io.ReadWriter
	Len() int
}

type CopyBodyStream struct {
	left  int
	buf   ReadWriterLen
	input io.ReadCloser
}

func NewCopyBodyStream(left int, buf ReadWriterLen, rc io.ReadCloser) *CopyBodyStream {
	return &CopyBodyStream{
		left:  left,
		buf:   buf,
		input: rc,
	}
}

func (cb *CopyBodyStream) Len() int {
	return cb.buf.Len()
}

func (cb *CopyBodyStream) Read(p []byte) (n int, err error) {
	n, err = cb.input.Read(p)
	if cb.left > 0 && n > 0 {
		m := min(n, cb.left)
		written, err := cb.buf.Write(p[:m])
		if err != nil {
			return 0, err
		}
		cb.left -= written
	}
	return n, err
}

func (cb *CopyBodyStream) Close() error {
	return cb.input.Close()
}

func (cb *CopyBodyStream) GetBody() io.ReadCloser {
	return io.NopCloser(cb.buf)
}
