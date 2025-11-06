package io

import (
	"bytes"
)

type mybuf struct {
	buf *bytes.Buffer
}

func (mybuf) Close() error {
	return nil
}

func (b mybuf) Read(p []byte) (int, error) {
	return b.buf.Read(p)
}
