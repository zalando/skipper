package sed

import (
	"bytes"
	"errors"
	"io"
	"regexp"
)

// TODO:
// - we need to document that only non-zero matches are replaced
// - we can document that it is currently not supported, together
//   with the templating

const (
	readBufferSize             = 8 << 10
	defaultMaxEditorBufferSize = 2 << 20
)

type editor struct {
	source        io.Reader
	pattern       *regexp.Regexp
	replacement   []byte
	delimiter     []byte
	maxBufferSize int
	prefix        []byte
	ready         []byte
	pending       []byte
	pendingMatch  bool
	readBuffer    []byte
	err           error
	closed        bool
}

var ErrClosed = errors.New("reader closed")

func newEditor(
	source io.Reader,
	pattern *regexp.Regexp,
	replacement []byte,
	delimiter []byte,
	maxBufferSize int,
) *editor {
	if maxBufferSize <= 0 {
		maxBufferSize = defaultMaxEditorBufferSize
	}

	prefix, _ := pattern.LiteralPrefix()
	return &editor{
		source:        source,
		pattern:       pattern,
		replacement:   replacement,
		delimiter:     delimiter,
		maxBufferSize: maxBufferSize,
		prefix:        []byte(prefix),
		readBuffer:    make([]byte, readBufferSize),
	}
}

func (e *editor) editChunk(chunk []byte) {
	for len(chunk) > 0 {
		match := e.pattern.FindIndex(chunk)
		if len(match) == 0 || match[0] == 0 && match[1] == 0 {
			e.ready = append(e.ready, chunk...)
			break
		}

		e.ready = append(e.ready, chunk[:match[0]]...)
		if match[1] > match[0] {
			e.ready = append(e.ready, e.replacement...)
		}

		chunk = chunk[match[1]:]
	}
}

func (e *editor) finalize() {
	if len(e.delimiter) == 0 {
		if e.pendingMatch {
			e.ready = e.replacement
		} else {
			e.ready = e.pending
		}
	} else {
		e.editChunk(e.pending)
	}

	e.pending = nil
	e.pendingMatch = false
}

func (e *editor) readSource(readSize int) int {
	var n, count int
	for i := 0; i < readSize; i++ {
		n, e.err = e.source.Read(e.readBuffer)
		e.pending = append(e.pending, e.readBuffer[:n]...)
		count += n
		if n == 0 || e.err != nil {
			break
		}
	}

	return count
}

func (e *editor) editUnbound() bool {
	var hasProcessed bool
	for {
		if len(e.prefix) > 0 && len(e.pending) >= len(e.prefix) {
			skip := bytes.Index(e.pending, e.prefix)
			if skip > 0 {
				e.ready = append(e.ready, e.pending[:skip]...)
				e.pending = e.pending[skip:]
				hasProcessed = true
			}
		}

		match := e.pattern.FindIndex(e.pending)
		if len(match) == 0 {
			e.pendingMatch = false
			break
		}

		if match[0] > 0 {
			hasProcessed = true
		}

		e.ready = append(e.ready, e.pending[:match[0]]...)
		e.pending = e.pending[match[0]:]
		match[1] -= match[0]
		match[0] = 0

		if match[1] == 0 {
			e.pendingMatch = false
			break
		}

		if match[1] == len(e.pending) {
			e.pendingMatch = true
			break
		}

		e.ready = append(e.ready, e.replacement...)
		e.pending = e.pending[match[1]:]
		hasProcessed = true
	}

	return hasProcessed
}

func (e *editor) editDelimited() bool {
	var hasProcessed bool
	for {
		endChunk := bytes.Index(e.pending, e.delimiter)
		if endChunk < 0 {
			break
		}

		hasProcessed = true
		chunk := e.pending[:endChunk+len(e.delimiter)]
		e.pending = e.pending[len(chunk):]
		e.editChunk(chunk)
	}

	return hasProcessed
}

func (e *editor) trimPending() {
	if len(e.delimiter) > 0 {
		e.editChunk(e.pending)
		e.pending = nil
		return
	}

	if e.pendingMatch {
		e.ready = append(e.ready, e.replacement...)
		e.pending = nil
		e.pendingMatch = false
		return
	}

	e.ready = append(e.ready, e.pending[:e.maxBufferSize]...)
	e.pending = e.pending[e.maxBufferSize:]
}

func (e *editor) Read(p []byte) (int, error) {
	if e.closed {
		return 0, ErrClosed
	}

	var count int
	readSize := 1
	for {
		n := copy(p, e.ready)
		p, e.ready = p[n:], e.ready[n:]
		count += n
		if len(p) == 0 {
			return count, nil
		}

		if e.err != nil {
			if len(e.pending) > 0 {
				e.finalize()
				continue
			}

			if count > 0 {
				return count, nil
			}

			return 0, e.err
		}

		if rcount := e.readSource(readSize); rcount == 0 {
			if e.err != nil {
				continue
			}

			return count, nil
		}

		readSize *= 2
		if len(e.delimiter) == 0 {
			if e.editUnbound() {
				readSize = 1
			}
		} else {
			if e.editDelimited() {
				readSize = 1
			}
		}

		if len(e.pending) > e.maxBufferSize {
			readSize = 1
			for len(e.pending) > e.maxBufferSize {
				e.trimPending()
			}
		}
	}
}

// It closed the undelrying reader.
func (e *editor) Close() error {
	e.closed = true
	if c, ok := e.source.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
