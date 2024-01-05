package io

import (
	"context"
	"io"
)

type logBody struct {
	ctx    context.Context
	fmtstr string
	log    func(format string, args ...interface{})
	input  io.ReadCloser
}

func newLogBody(ctx context.Context, fmtstr string, log func(format string, args ...interface{}), rc io.ReadCloser) io.ReadCloser {
	return &logBody{
		ctx:    ctx,
		fmtstr: fmtstr,
		input:  rc,
		log:    log,
	}
}

func (lb *logBody) Read(p []byte) (int, error) {
	n, err := lb.input.Read(p)
	if n > 0 {
		lb.log("%s%s", lb.fmtstr, p)
	}
	return n, err
}

func (lb *logBody) Close() error {
	return lb.input.Close()
}

func LogBody(ctx context.Context, fmtstr string, log func(format string, args ...interface{}), rc io.ReadCloser) io.ReadCloser {
	return newLogBody(ctx, fmtstr, log, rc)
}
