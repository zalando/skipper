package iotest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestSlowWriter(t *testing.T) {
	for _, tt := range []struct {
		name    string
		w       FlushedWriter
		in      string
		wantErr bool
		err     error
	}{
		{
			name:    "test flushedwriter",
			w:       newFlushedWriter(bytes.NewBuffer(make([]byte, 10))),
			in:      "hello",
			wantErr: false,
		},
		{
			name:    "test slowwriter with buffer",
			w:       NewSlowWriter(newFlushedWriter(bytes.NewBuffer(make([]byte, 10))), time.Millisecond),
			in:      "hello",
			wantErr: false,
		},
		{
			name:    "test slowwriter truncate writer",
			w:       NewSlowWriter(newFlushedWriter(iotest.TruncateWriter(bytes.NewBuffer(make([]byte, 10)), 2)), time.Millisecond),
			in:      "hello",
			wantErr: false,
		},
		{
			name:    "test slowwriter timeout writer",
			w:       NewSlowWriter(newFlushedWriter(NewTimeoutWriter(bytes.NewBuffer(make([]byte, 10)), 2)), time.Millisecond),
			in:      "hello",
			wantErr: true,
			err:     iotest.ErrTimeout,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			n, err := tt.w.Write([]byte(tt.in))
			if tt.wantErr {
				switch err {
				case nil:
					t.Fatal("Failed to trigger error")
				case tt.err:
					t.Logf("Got expected error %v", err)
					return
				}
			}
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to write: %v", err)
			}
			if n != len(tt.in) {
				t.Fatalf("Failed to write all bytes want: %d, got %d", len(tt.in), n)
			}
		})
	}

}

func TestSlowResponseWriter(t *testing.T) {
	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		sw := NewSlowResponseWriter(w, 1*time.Millisecond) // each byte 1ms
		sw.WriteHeader(200)
		sw.Flush()

		from := bytes.NewBufferString(strings.Repeat("A", 100)) // 100B
		b := make([]byte, 1024)
		io.CopyBuffer(sw, from, b) // takes >100ms
	}))
	defer slowBackend.Close()

	req, err := http.NewRequest("GET", slowBackend.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	timeBeforeRequest := time.Now()
	rsp, err := slowBackend.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("Failed to get response, want %d, got %d", 200, rsp.StatusCode)
	}

	io.Copy(io.Discard, rsp.Body)
	if d := time.Since(timeBeforeRequest); d < 100*time.Millisecond {
		t.Fatalf("Failed to have slow response %v < %v", d, 100*time.Millisecond)
	}

}
