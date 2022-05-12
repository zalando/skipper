package block

import (
	"bytes"
	"errors"
	"io"

	log "github.com/sirupsen/logrus"
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

// editor provides a reader that wraps an input reader, and replaces each occurence of
// the provided search pattern with the provided replacement. It can be used with a
// delimiter or without.
//
// When using it with a delimiter, it reads enough data from the input until meeting
// a delimiter or reaching maxBufferSize. The chunk includes the delimiter if any. Then
// every occurence of the pattern is replaced, and the entire edited chunk is returned
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
//
type matcher struct {

	// init:
	input             io.ReadCloser
	matchList         []string
	maxBufferSize     int
	maxBufferHandling maxBufferHandling
	readBuffer        []byte

	// state:
	ready   *bytes.Buffer
	pending *bytes.Buffer

	// final:
	err    error
	closed bool
}

var (
	ErrEditorBufferFull = errors.New("editor buffer full")
)

func newMatcher(
	input io.ReadCloser,
	matchList []string,
	maxBufferSize int,
	mbh maxBufferHandling,
) *matcher {
	if maxBufferSize <= 0 {
		maxBufferSize = defaultMaxEditorBufferSize
	}

	rsize := readBufferSize
	if maxBufferSize < rsize {
		rsize = maxBufferSize
	}

	return &matcher{
		input:             input,
		matchList:         matchList,
		maxBufferSize:     maxBufferSize,
		maxBufferHandling: mbh,
		readBuffer:        make([]byte, rsize),
		pending:           bytes.NewBuffer(nil),
		ready:             bytes.NewBuffer(nil),
	}
}

func (m *matcher) readNTimes(times int) (bool, error) {
	var consumedInput bool
	for i := 0; i < times; i++ {
		n, err := m.input.Read(m.readBuffer)
		m.pending.Write(m.readBuffer[:n])
		if n > 0 {
			consumedInput = true
		}

		if err != nil {
			return consumedInput, err
		}

	}

	return consumedInput, nil
}

func (m *matcher) match(b []byte) (int, error) {
	var consumed int

	for _, s := range m.matchList {
		if bytes.Contains(b, []byte(s)) {
			b = nil
			log.Errorf("Content blocked: %v", ErrBlocked)
			return 0, ErrBlocked
		}
	}
	consumed += len(b)
	return consumed, nil

}

func (m *matcher) fill(requested int) error {
	readSize := 1
	for m.ready.Len() < requested {
		consumedInput, err := m.readNTimes(readSize)
		if !consumedInput {
			io.CopyBuffer(m.ready, m.pending, m.readBuffer)
			return err
		}

		if m.pending.Len() > m.maxBufferSize {
			switch m.maxBufferHandling {
			case maxBufferAbort:
				return ErrEditorBufferFull
			default:
				_, err := m.match(m.pending.Bytes())
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

	if m.err == ErrEditorBufferFull {
		return 0, ErrEditorBufferFull
	}

	if m.err == ErrBlocked {
		return 0, ErrBlocked
	}

	n, _ := m.ready.Read(p)

	if n == 0 && len(p) > 0 && m.err != nil {
		return 0, m.err
	}

	n, err := m.match(p)

	if err != nil {
		m.closed = true
		return 0, err
	}

	return n, nil
}

// Closes closes the undelrying reader if it implements io.Closer.
func (m *matcher) Close() error {
	m.closed = true
	if c, ok := m.input.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
