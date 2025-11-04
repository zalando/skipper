package iotest

import (
	"bytes"
	"io"
	"testing"
	"testing/iotest"
	"time"
)

func TestSlowReader(t *testing.T) {
	for _, tt := range []struct {
		name    string
		r       io.Reader
		in      string
		wantErr bool
		err     error
	}{
		{
			name:    "test slowreader",
			r:       bytes.NewBufferString("hello"),
			in:      "hello",
			wantErr: false,
		},
		{
			name:    "test slowreader TimeoutReader ",
			r:       iotest.TimeoutReader(bytes.NewBufferString("hello")),
			in:      "hello",
			wantErr: true,
			err:     iotest.ErrTimeout,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			sr := NewSlowReader(tt.r, 1*time.Millisecond)
			n, err := io.Copy(io.Discard, sr)
			if !tt.wantErr && err != nil {
				t.Fatalf("Failed to copy got err: %v", err)
			}
			if tt.wantErr {
				switch err {
				case nil:
					t.Fatal("Failed to get an error")
				case tt.err:
					t.Logf("Got expected error %v", err)
					return
				}
			}
			if int(n) != len(tt.in) {
				t.Fatalf("Failed to read %d bytes, read %d", len(tt.in), n)
			}

		})
	}

}
