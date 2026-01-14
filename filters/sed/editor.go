package sed

import (
	"bytes"
	"errors"
	"io"
	"regexp"
)

const (
	readBufferSize             = 8192
	defaultMaxEditorBufferSize = 2097152 // 2Mi
)

type maxBufferHandling int

const (
	maxBufferBestEffort maxBufferHandling = iota
	maxBufferAbort
)

// editor provides a reader that wraps an input reader, and replaces each occurrence of
// the provided search pattern with the provided replacement. It can be used with a
// delimiter or without.
//
// When using it with a delimiter, it reads enough data from the input until meeting
// a delimiter or reaching maxBufferSize. The chunk includes the delimiter if any. Then
// every occurrence of the pattern is replaced, and the entire edited chunk is returned
// to the caller.
//
// When not using a delimiter, it reads enough data until at least a complete match of the
// pattern is met or the maxBufferSize is reached. When the pattern matches the entire
// buffered input, the replaced content is returned to the caller when maxBufferSize is
// reached. This also means that more replacements can happen than if we edited the
// entire content in one piece, but this is necessary to be able to use the editor for
// input with unknown length.
//
// When the maxBufferHandling is set to maxBufferAbort, then the streaming is aborted
// and the rest of the payload is dropped.
//
// To limit the number of repeated scans over the buffered data, the size of the
// additional data read from the input grows exponentially with every iteration that
// didn't result with any edited data returned to the caller. If there was any edited
// returned to the caller, the read size is reset to the initial value.
//
// When the input returns an error, e.g. EOF, the editor finishes editing the buffered
// data, returns it to the caller, and returns the received error on every subsequent
// read.
//
// When the editor is closed, it doesn't read anymore from the input or return any
// buffered data. If the input implements io.Closer, closing the editor closes the
// input, too.
type editor struct {
	// init:
	input             io.Reader
	pattern           *regexp.Regexp
	replacement       []byte
	delimiter         []byte
	maxBufferSize     int
	maxBufferHandling maxBufferHandling
	prefix            []byte
	readBuffer        []byte

	// state:
	ready   *bytes.Buffer
	pending *bytes.Buffer

	// final:
	err    error
	closed bool
}

var (
	ErrClosed           = errors.New("reader closed")
	ErrEditorBufferFull = errors.New("editor buffer full")
)

func newEditor(
	input io.Reader,
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
		input:             input,
		pattern:           pattern,
		replacement:       replacement,
		delimiter:         delimiter,
		maxBufferSize:     maxBufferSize,
		maxBufferHandling: mbh,
		prefix:            []byte(prefix),
		readBuffer:        make([]byte, rsize),
		pending:           bytes.NewBuffer(nil),
		ready:             bytes.NewBuffer(nil),
	}
}

func (e *editor) readNTimes(times int) (bool, error) {
	var consumedInput bool
	for i := 0; i < times; i++ {
		n, err := e.input.Read(e.readBuffer)
		e.pending.Write(e.readBuffer[:n])
		if n > 0 {
			consumedInput = true
		}

		if err != nil {
			return consumedInput, err
		}
	}

	return consumedInput, nil
}

func (e *editor) edit(b []byte, keepLastChunk bool) (int, bool) {
	var consumed int
	for len(b) > 0 {
		if len(e.prefix) > 0 && len(b) >= len(e.prefix) {
			skip := bytes.Index(b, e.prefix)
			if skip > 0 {
				e.ready.Write(b[:skip])
				consumed += skip
				b = b[skip:]
			}
		}

		match := e.pattern.FindIndex(b)
		if len(match) == 0 {
			if keepLastChunk {
				return consumed, false
			}

			e.ready.Write(b)
			consumed += len(b)
			return consumed, false
		}

		e.ready.Write(b[:match[0]])
		consumed += match[0]

		if match[1] == match[0] {
			if keepLastChunk {
				return consumed, false
			}

			e.ready.Write(b[match[0]:])
			consumed += len(b) - match[0]
			return consumed, false
		}

		if keepLastChunk && match[1] == len(b) {
			return consumed, true
		}

		e.ready.Write(e.replacement)
		consumed += match[1] - match[0]
		b = b[match[1]:]
	}

	return consumed, false
}

func (e *editor) editUnbound() bool {
	consumed, pendingMatches := e.edit(e.pending.Bytes(), true)
	e.pending.Next(consumed)
	return pendingMatches
}

func (e *editor) editDelimited() {
	for {
		endChunk := bytes.Index(e.pending.Bytes(), e.delimiter)
		if endChunk < 0 {
			return
		}

		chunk := e.pending.Next(endChunk + len(e.delimiter))
		e.edit(chunk, false)
	}
}

func (e *editor) finalizeEdit(pendingMatches bool) {
	if pendingMatches {
		e.ready.Write(e.replacement)
		return
	}

	if len(e.delimiter) == 0 {
		io.CopyBuffer(e.ready, e.pending, e.readBuffer)
		return
	}

	e.edit(e.pending.Bytes(), false)
}

func (e *editor) fill(requested int) error {
	var pendingMatches bool
	readSize := 1
	for e.ready.Len() < requested {
		consumedInput, err := e.readNTimes(readSize)
		if !consumedInput {
			if err != nil {
				e.finalizeEdit(pendingMatches)
			}

			return err
		}

		if len(e.delimiter) == 0 {
			pendingMatches = e.editUnbound()
		} else {
			e.editDelimited()
		}

		if err != nil {
			e.finalizeEdit(pendingMatches)
			return err
		}

		if e.pending.Len() > e.maxBufferSize {
			switch e.maxBufferHandling {
			case maxBufferAbort:
				return ErrEditorBufferFull
			default:
				e.edit(e.pending.Bytes(), false)
				e.pending.Reset()
				readSize = 1
			}
		}

		readSize *= 2
	}

	return nil
}

func (e *editor) Read(p []byte) (int, error) {
	if e.closed {
		return 0, ErrClosed
	}

	if e.ready.Len() == 0 && e.err != nil {
		return 0, e.err
	}

	if e.ready.Len() < len(p) {
		e.err = e.fill(len(p))
	}

	if e.err == ErrEditorBufferFull {
		return 0, ErrEditorBufferFull
	}

	n, _ := e.ready.Read(p)
	if n == 0 && len(p) > 0 && e.err != nil {
		return 0, e.err
	}

	return n, nil
}

// Closes closes the underlying reader if it implements io.Closer.
func (e *editor) Close() error {
	e.closed = true
	if c, ok := e.input.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
