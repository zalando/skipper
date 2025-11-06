package iotest

import (
	"io"
	"time"
)

type SlowReader struct {
	r io.Reader
	d time.Duration
}

// NewSlowReader creates a new slowReader wrapping the given io.Reader
// with a delay after each byte.
func NewSlowReader(r io.Reader, d time.Duration) *SlowReader {
	return &SlowReader{
		r: r,
		d: d,
	}
}

// Read implements the io.Reader interface.
// It reads one byte at a time from the underlying reader,
// sleeps for the specified Delay, and populates the provided buffer 'p'.
func (sr *SlowReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for i := range len(p) {
		oneByte := make([]byte, 1)
		bytesRead, err := sr.r.Read(oneByte)

		if err != nil {
			if n > 0 && err == io.EOF {
				return n, nil
			}
			return n, err
		}

		// single byte read
		if bytesRead == 1 {
			p[i] = oneByte[0]
			n++
			time.Sleep(sr.d)
		} else {
			return n, nil
		}
	}

	return n, nil
}
