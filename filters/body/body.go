package body

import (
	"bytes"
	"errors"
	"io"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

var (
	ErrClosed  = errors.New("reader closed")
	ErrBlocked = errors.New("blocked string match found in body")
)

type bodyMatchSpec struct{}

type bodyMatch struct {
	match []string
}

func NewBodyMatchFilter() filters.Spec {
	return &bodyMatchSpec{}
}

func (*bodyMatchSpec) Name() string {
	return filters.BodyMatchName
}

func (*bodyMatchSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	sargs := make([]string, 0, len(args))
	for _, w := range args {
		switch v := w.(type) {
		case string:
			sargs = append(sargs, string(v))
		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &bodyMatch{
		match: sargs,
	}, nil
}

func (bm *bodyMatch) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req.ContentLength == 0 {
		return
	}
	req.Body = newBodyMatchBuffer(req.Body, bm.match)
}

func (*bodyMatch) Response(filters.FilterContext) {}

type bodyMatchBuffer struct {
	input  io.ReadCloser
	closed bool
	match  []string
}

func newBodyMatchBuffer(rc io.ReadCloser, match []string) *bodyMatchBuffer {
	return &bodyMatchBuffer{
		input:  rc,
		match:  match,
		closed: false,
	}
}
func (bmb *bodyMatchBuffer) Read(p []byte) (int, error) {

	if bmb.closed {
		return 0, ErrClosed
	}

	n, err := bmb.input.Read(p)

	if err != nil && err != io.EOF {
		log.Errorf("bodyMatchBuffer: Failed to read body: %v", err)
		return 0, err
	}

	for _, s := range bmb.match {
		if bytes.Contains(p, []byte(s)) {
			p = nil
			return 0, ErrBlocked
		}
	}

	return n, err
}

func (bmb *bodyMatchBuffer) Close() error {
	if bmb.closed {
		return nil
	}
	bmb.closed = true
	return bmb.input.Close()
}
