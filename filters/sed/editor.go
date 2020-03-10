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

type maxBufferHandling int

const (
	maxBufferBestEffort maxBufferHandling = iota
	maxBufferAbort
)

// editor provides a reader that wraps a source reader, and replaces each occurence of
// the provided search pattern with the provided replacement. It can be used with a
// delimiter or without.
//
// When using it with a delimiter, it reads enough data from the source until meeting
// a delimiter or reaching maxBufferSize. The chunk includes the delimiter if any. Then
// every occurence of the pattern is replaced, and the entire edited chunk is returned
// to the caller.
//
// When not using a delimiter, it reads enough data until at least complete match of the
// pattern is met or the maxBufferSize is reached. When the pattern matches the entire
// buffered input, the replaced content is returned to the caller when maxBufferSize is
// reached. This also means that more replacements can happen than if we edited the
// entire content in one piece, but this is necessary to be able to use the editor for
// input with unknown length.
//
// To limit the number of repeated scans over the buffered data, the size of the
// additional data read from the source grows exponentially with every iteration that
// didn't result with any edited data returned to the caller. If there was any edited
// returned to the caller, the read size is reset to the initial value.
//
// When the source returns an error, e.g. EOF, the editor finishes editing the buffered
// data, returns it to the caller, and returns the received error on every subsequent
// read.
//
// When the editor is closed, it doesn't read anymore from the source or return any
// buffered data. If the source implements io.Closer, closing the editor closes the
// source, too.
//
type editor struct {
	source            io.Reader
	pattern           *regexp.Regexp
	replacement       []byte
	delimiter         []byte
	maxBufferSize     int
	maxBufferHandling maxBufferHandling
	prefix            []byte
	ready             []byte
	pending           []byte
	pendingMatch      bool
	readBuffer        []byte
	err               error
	closed            bool
}

var (
	ErrClosed           = errors.New("reader closed")
	ErrEditorBufferFull = errors.New("editor buffer full")
)

func newEditor(
	source io.Reader,
	pattern *regexp.Regexp,
	replacement []byte,
	delimiter []byte,
	maxBufferSize int,
	mbh maxBufferHandling,
) *editor {
	if maxBufferSize <= 0 {
		maxBufferSize = defaultMaxEditorBufferSize
	}

	rsize := readBufferSize
	if maxBufferSize < rsize {
		rsize = maxBufferSize
	}

	prefix, _ := pattern.LiteralPrefix()
	return &editor{
		source:            source,
		pattern:           pattern,
		replacement:       replacement,
		delimiter:         delimiter,
		maxBufferSize:     maxBufferSize,
		maxBufferHandling: mbh,
		prefix:            []byte(prefix),
		readBuffer:        make([]byte, rsize),
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

func (e *editor) processPendingForced() {
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
			if len(e.pending) > 0 && e.err != ErrEditorBufferFull {
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
			switch e.maxBufferHandling {
			case maxBufferAbort:
				e.err = ErrEditorBufferFull
			default:
				readSize = 1
				for len(e.pending) > e.maxBufferSize {
					// TODO: trim is not a good name
					e.processPendingForced()
				}
			}
		}
	}
}

// It closes the undelrying reader.
func (e *editor) Close() error {
	e.closed = true
	if c, ok := e.source.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
