package builtin

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func gzipped(t *testing.T, payload string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write gzip data: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return &buf
}

func newDecompressRequestContext(t *testing.T, body io.Reader, encoding string) *filtertest.Context {
	t.Helper()
	req, err := http.NewRequest("POST", "http://example.com", body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	return &filtertest.Context{
		FRequest:  req,
		FStateBag: make(map[string]interface{}),
	}
}

func TestDecompressRequestGzip(t *testing.T) {
	ctx := newDecompressRequestContext(t, gzipped(t, "test payload"), "gzip")

	f, err := NewDecompressRequest().CreateFilter(nil)
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}
	f.Request(ctx)

	if got := ctx.Request().Header.Get("Content-Encoding"); got != "" {
		t.Errorf("expected Content-Encoding header to be removed, got: %q", got)
	}
	if ctx.Request().ContentLength != -1 {
		t.Errorf("expected ContentLength to be -1, got: %d", ctx.Request().ContentLength)
	}

	got, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		t.Fatalf("failed to read decompressed body: %v", err)
	}
	if string(got) != "test payload" {
		t.Errorf("expected decompressed body to be %q, got: %q", "test payload", string(got))
	}

	if _, ok := ctx.StateBag()[DecompressionNotPossible]; ok {
		t.Error("did not expect DecompressionNotPossible to be set")
	}
}

func TestDecompressRequestNoEncoding(t *testing.T) {
	body := bytes.NewBufferString("plain")
	ctx := newDecompressRequestContext(t, body, "")

	f, err := NewDecompressRequest().CreateFilter(nil)
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}
	f.Request(ctx)

	got, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(got) != "plain" {
		t.Errorf("expected body unchanged, got: %q", string(got))
	}
	if _, ok := ctx.StateBag()[DecompressionNotPossible]; ok {
		t.Error("did not expect DecompressionNotPossible to be set")
	}
}

func TestDecompressRequestUnsupportedEncoding(t *testing.T) {
	ctx := newDecompressRequestContext(t, bytes.NewBufferString("x"), "unsupported")

	f, err := NewDecompressRequest().CreateFilter(nil)
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}
	f.Request(ctx)

	if _, ok := ctx.StateBag()[DecompressionNotPossible]; !ok {
		t.Error("expected DecompressionNotPossible to be set in state bag")
	}
	if got := ctx.Request().Header.Get("Content-Encoding"); got != "unsupported" {
		t.Errorf("expected Content-Encoding to be preserved when unsupported, got: %q", got)
	}
}

func TestDecompressRequestInvalidPayload(t *testing.T) {
	// claim gzip but send non-gzip data; gzip.Reader.Reset will fail during init.
	ctx := newDecompressRequestContext(t, bytes.NewBufferString("not gzip data"), "gzip")

	f, err := NewDecompressRequest().CreateFilter(nil)
	if err != nil {
		t.Fatalf("failed to create filter: %v", err)
	}
	f.Request(ctx)

	if _, ok := ctx.StateBag()[DecompressionNotPossible]; !ok {
		t.Error("expected DecompressionNotPossible to be set on init error")
	}
	if _, ok := ctx.StateBag()[DecompressionError]; !ok {
		t.Error("expected DecompressionError to be set on init error")
	}
}

func TestDecompressRequestName(t *testing.T) {
	if got := NewDecompressRequest().Name(); got != "decompressRequest" {
		t.Errorf("unexpected filter name: %q", got)
	}
}
