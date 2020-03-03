package sed

import (
	"bytes"
	"errors"
	"io"
	"regexp"
)

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
	pendingMatch  []int
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

func (b *editor) Read(p []byte) (int, error) {
	if b.closed {
		return 0, ErrClosed
	}

	var count int
	readSize := 1
	for {
		// If we have something ready, return it.
		n := copy(p, b.ready)
		p, b.ready = p[n:], b.ready[n:]
		count += n
		if len(p) == 0 {
			return count, nil
		}

		// If we received an error from the underlying reader, e.g. EOF,
		// check if we still can return something, or return the error.
		//
		// Not mandatory, but let's postpone the error until nothing else
		// was left.
		//
		// We can have pending data only in two possible states at this point:
		//
		// - no match -> return the data
		// - match full -> return the replacement
		//
		if b.err != nil {
			if len(b.pending) == 0 {
				if count > 0 {
					return count, nil
				}

				return 0, b.err
			}

			if len(b.pendingMatch) != 0 {
				b.ready = b.replacement
			} else {
				b.ready = b.pending
			}

			b.pending = nil
			b.pendingMatch = nil
			continue
		}

		// There is no more data ready, we need to read more input.
		var rn, rcount int
		for i := 0; i < readSize; i++ {
			rn, b.err = b.source.Read(b.readBuffer)
			b.pending = append(b.pending, b.readBuffer[:rn]...)
			rcount += rn
			if rn == 0 || b.err != nil {
				break
			}
		}

		// If there was no data on the input, we may have received an
		// error, or the input is simply non-blocking. In the former
		// case, continue with the read error handling. In the latter
		// case, return what could read, and leave the decision to the
		// consumer how to proceed.
		if rcount == 0 {
			if b.err != nil {
				continue
			}

			return count, nil
		}

		// In case we couldn't process any data, we increase the read size
		// exponentially. This way we can limit the number repeated scans
		// of the same data from N to logN.
		readSize *= 2

		if len(b.delimiter) == 0 {
			for {
				// Scanning for the known prefix, if there is any, prevents
				// unnecessary scanning of the pending data when the editor
				// buffer grows large.
				if len(b.prefix) > 0 {
					skip := bytes.Index(b.pending, b.prefix)
					if skip > 0 {
						b.ready = append(b.ready, b.pending[:skip]...)
						b.pending = b.pending[skip:]
						readSize = 1
					} else if skip < 0 {
						b.ready = append(b.ready, b.pending...)
						b.pending = nil
						b.pendingMatch = nil
						readSize = 1
						break
					}
				}

				b.pendingMatch = b.pattern.FindIndex(b.pending)
				if len(b.pendingMatch) == 0 {
					break
				}

				// We reset the read size to the default, if we know that we
				// can process some data.
				if b.pendingMatch[0] > 0 {
					readSize = 1
				}

				b.ready = append(b.ready, b.pending[:b.pendingMatch[0]]...)
				b.pending = b.pending[b.pendingMatch[0]:]
				b.pendingMatch[1] -= b.pendingMatch[0]
				b.pendingMatch[0] = 0

				// TODO: we need to document that only non-zero matches are replaced
				if b.pendingMatch[1] == 0 {
					b.pendingMatch = nil
					break
				}

				if b.pendingMatch[1] == len(b.pending) {
					break
				}

				b.ready = append(b.ready, b.replacement...)
				b.pending = b.pending[b.pendingMatch[1]:]
				readSize = 1
			}
		} else {
			for {
				endChunk := bytes.Index(b.pending, b.delimiter)
				if endChunk < 0 {
					break
				}

				// When we have a non-zero delimited chunk, we always process
				// it before reading more data from the underlying reader.
				readSize = 1

				// Include the delimiter, this way it can be controlled
				// in the expression whether to keep it or not.
				// TODO: test both cases
				chunk := b.pending[:endChunk+len(b.delimiter)]

				for len(chunk) > 0 {
					match := b.pattern.FindIndex(chunk)
					if len(match) == 0 || match[0] == 0 && match[1] == 0 {
						b.ready = append(b.ready, chunk...)
						break
					}

					b.ready = append(b.ready, chunk[:match[0]]...)
					if match[1] > match[0] {
						b.ready = append(b.ready, b.replacement...)
					}

					chunk = chunk[match[1]:]
				}
			}
		}

		for len(b.pending) > b.maxBufferSize {
			if len(b.pendingMatch) == 0 {
				b.ready = append(b.ready, b.pending[:b.maxBufferSize]...)
				b.pending = b.pending[b.maxBufferSize:]
			} else {
				b.ready = append(b.ready, b.replacement...)
				b.pending = nil
				b.pendingMatch = nil
			}
		}
	}
}

// It closed the undelrying reader.
func (b editor) Close() error {
	if c, ok := b.source.(io.Closer); ok {
		return c.Close()
	}

	b.closed = true
	return nil
}
