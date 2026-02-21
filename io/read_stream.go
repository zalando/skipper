package io

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
)

var (
	ErrClosed  = errors.New("reader closed")
	ErrBlocked = errors.New("blocked string match found in stream")
)

const (
	defaultReadBufferSize uint64 = 8192
)

type MaxBufferHandling int

const (
	MaxBufferBestEffort MaxBufferHandling = iota
	MaxBufferAbort
)

type matcher struct {
	ctx               context.Context
	once              sync.Once
	input             io.ReadCloser
	f                 func([]byte) (int, error)
	maxBufferSize     uint64
	maxBufferHandling MaxBufferHandling
	readBuffer        []byte

	ready   *bytes.Buffer
	pending *bytes.Buffer

	err    error
	closed bool
}

var (
	ErrMatcherBufferFull = errors.New("matcher buffer full")
)

func newMatcher(
	ctx context.Context,
	input io.ReadCloser,
	f func([]byte) (int, error),
	maxBufferSize uint64,
	mbh MaxBufferHandling,
) *matcher {

	rsize := min(maxBufferSize, defaultReadBufferSize)

	return &matcher{
		ctx:               ctx,
		once:              sync.Once{},
		input:             input,
		f:                 f,
		maxBufferSize:     maxBufferSize,
		maxBufferHandling: mbh,
		readBuffer:        make([]byte, rsize),
		pending:           bytes.NewBuffer(nil),
		ready:             bytes.NewBuffer(nil),
	}
}

func (m *matcher) readNTimes(times int) (bool, error) {
	var consumedInput bool
	for range times {
		n, err := m.input.Read(m.readBuffer)
		_, err2 := m.pending.Write(m.readBuffer[:n])

		if n > 0 {
			consumedInput = true
		}
		if err != nil {
			return consumedInput, err
		}
		if err2 != nil {
			return consumedInput, err2
		}
	}
	return consumedInput, nil
}

func (m *matcher) fill(requested int) error {
	readSize := 1
	for m.ready.Len() < requested {
		consumedInput, err := m.readNTimes(readSize)
		if !consumedInput {
			io.CopyBuffer(m.ready, m.pending, m.readBuffer)
			return err
		}

		if uint64(m.pending.Len()) > m.maxBufferSize {
			switch m.maxBufferHandling {
			case MaxBufferAbort:
				return ErrMatcherBufferFull
			default:
				select {
				case <-m.ctx.Done():
					m.Close()
					return m.ctx.Err()
				default:
				}
				_, err := m.f(m.pending.Bytes())
				if err != nil {
					return err
				}
				m.pending.Reset()
				readSize = 1
			}
		}

		readSize *= 2
	}
	return nil
}

func (m *matcher) Read(p []byte) (int, error) {
	if m.closed {
		return 0, ErrClosed
	}

	if m.ready.Len() == 0 && m.err != nil {
		return 0, m.err
	}

	if m.ready.Len() < len(p) {
		m.err = m.fill(len(p))
	}

	switch m.err {
	case ErrMatcherBufferFull, ErrBlocked:
		return 0, m.err
	}

	n, _ := m.ready.Read(p)
	if n == 0 && len(p) > 0 && m.err != nil {
		return 0, m.err
	}
	p = p[:n]

	select {
	case <-m.ctx.Done():
		m.Close()
		return 0, m.ctx.Err()
	default:
	}

	n, err := m.f(p)
	if err != nil {
		m.closed = true
		return 0, err
	}
	return n, nil
}

// Close closes the underlying reader if it implements io.Closer.
func (m *matcher) Close() error {
	var err error
	m.once.Do(func() {
		m.closed = true
		if c, ok := m.input.(io.Closer); ok {
			err = c.Close()
		}
	})
	return err
}

/*
   Wants:
    - [x] filters can read the body content for example WAF scoring
    - [ ] filters can change the body content for example sedRequest()
    - [x] filters need to be chainable (support -> )
    - [x] filters need to be able to stop streaming to request blockContent() or WAF deny()

   TODO(sszuecs):

   1) major optimization: use registry pattern and have only one body
   wrapped for concatenating readers and run all f() in a loop, so
   streaming does not happen for all but once for all
   readers. Important if one write is between two readers we cannot
   do this, so we need to detect this case.

   3) in case we ErrBlock, then we break the loop or cancel the
   context to stop processing. The registry control layer should be
   able to stop all processing.

*/

type BufferOptions struct {
	MaxBufferHandling MaxBufferHandling
	ReadBufferSize    uint64
}

// InspectReader wraps the given ReadCloser such that the given
// function f can inspect the streaming while streaming to the
// target. A target can be any io.ReadCloser, so for example the
// request body to the backend or the response body to the
// client. InspectReader applies given BufferOptions to the matcher.
//
// NOTE: This function is *experimental* and will likely change or disappear in the future.
func InspectReader(ctx context.Context, bo BufferOptions, f func([]byte) (int, error), rc io.ReadCloser) io.ReadCloser {
	if bo.ReadBufferSize < 1 {
		bo.ReadBufferSize = defaultReadBufferSize
	}
	return newMatcher(ctx, rc, f, bo.ReadBufferSize, bo.MaxBufferHandling)
}
