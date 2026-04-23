package io

import (
	"bytes"
	"fmt"
)

type failingWriter struct {
	buf *bytes.Buffer
}

func (*failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("failed to write")
}

func (fw *failingWriter) Read(p []byte) (int, error) {
	return fw.buf.Read(p)
}

func (fw *failingWriter) Len() int {
	return fw.buf.Len()
}
