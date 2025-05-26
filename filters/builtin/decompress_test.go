package builtin

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func backend(t *testing.T, contentEncoding string, content io.Reader) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentEncoding != "" {
			w.Header().Set("Content-Encoding", contentEncoding)
			w.Header().Set("Vary", "Accept-Encoding")
		}

		if _, err := io.Copy(w, content); err != nil {
			t.Fatal(err)
		}
	}))
}

func decompressingProxy(t *testing.T, backendURL string) *proxytest.TestProxy {
	routes := `
		* -> decompress() -> "%s"
	`
	r := eskip.MustParse(fmt.Sprintf(routes, backendURL))

	fr := make(filters.Registry)
	fr.Register(NewDecompress())
	return proxytest.New(fr, r[0])
}

func request(t *testing.T, url string) (status int, h http.Header, content string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Accept-Encoding", "deflate, gzip, br")
	c := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	rsp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
		return
	}

	defer rsp.Body.Close()
	status = rsp.StatusCode
	if rsp.StatusCode != http.StatusOK {
		return
	}

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
		return
	}

	content = string(b)
	return
}

func compressedBody(t *testing.T, content io.Reader, enc string) io.Reader {
	type compressor interface {
		io.WriteCloser
		Flush() error
	}

	var (
		b   bytes.Buffer
		c   compressor
		err error
	)

	check := func() {
		if err != nil {
			t.Fatal(err)
		}
	}

	switch enc {
	case "gzip":
		c = gzip.NewWriter(&b)
	case "br":
		c = brotli.NewWriter(&b)
	default:
		c, err = flate.NewWriter(&b, flate.DefaultCompression)
	}

	check()

	_, err = io.Copy(c, content)
	check()

	err = c.Flush()
	check()

	err = c.Close()
	check()

	return &b
}

func TestDecompress(t *testing.T) {
	t.Run("not compressed", func(t *testing.T) {
		b := backend(t, "", strings.NewReader("Hello, world!"))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, _, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if content != "Hello, world!" {
			t.Error("Failed to return the content unchanged.")
		}
	})

	t.Run("cannot decompress", func(t *testing.T) {
		b := backend(t, "unsupported", bytes.NewReader([]byte{1, 2, 3}))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, _, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if content != string([]byte{1, 2, 3}) {
			t.Error("Failed to return the content unchanged.")
		}
	})

	t.Run("decompress fails, after headers sent", func(t *testing.T) {
		b := backend(t, "gzip", bytes.NewReader([]byte{1, 2, 3}))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, _, _ := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}
	})

	t.Run("decompress, deflate", func(t *testing.T) {
		b := backend(t, "deflate", compressedBody(t, bytes.NewBufferString("Hello, world!"), "deflate"))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, h, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if _, has := h["Content-Encoding"]; has {
			t.Error("Failed to remove Content-Encoding header")
		}

		if _, has := h["Vary"]; has {
			t.Error("Failed to remove Vary header")
		}

		if content != "Hello, world!" {
			t.Error("Failed to return the content unchanged.")
		}
	})

	t.Run("decompress, gzip", func(t *testing.T) {
		b := backend(t, "gzip", compressedBody(t, bytes.NewBufferString("Hello, world!"), "gzip"))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, h, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if _, has := h["Content-Encoding"]; has {
			t.Error("Failed to remove Content-Encoding header")
		}

		if _, has := h["Vary"]; has {
			t.Error("Failed to remove Vary header")
		}

		if content != "Hello, world!" {
			t.Error("Failed to return the content unchanged.")
		}
	})

	t.Run("decompress, br", func(t *testing.T) {
		b := backend(t, "br", compressedBody(t, bytes.NewBufferString("Hello, world!"), "br"))
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, h, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if _, has := h["Content-Encoding"]; has {
			t.Error("Failed to remove Content-Encoding header")
		}

		if _, has := h["Vary"]; has {
			t.Error("Failed to remove Vary header")
		}

		if content != "Hello, world!" {
			t.Error("Failed to return the content unchanged.")
		}
	})

	t.Run("decompress, multiple compressions", func(t *testing.T) {
		var body io.Reader
		body = bytes.NewBufferString("Hello, world!")
		body = compressedBody(t, body, "deflate")
		body = compressedBody(t, body, "gzip")
		b := backend(t, "deflate, gzip", body)
		defer b.Close()

		p := decompressingProxy(t, b.URL)
		defer p.Close()

		status, h, content := request(t, p.URL)
		if status != http.StatusOK {
			t.Error(status)
		}

		if _, has := h["Content-Encoding"]; has {
			t.Error("Failed to remove Content-Encoding header")
		}

		if _, has := h["Vary"]; has {
			t.Error("Failed to remove Vary header")
		}

		if content != "Hello, world!" {
			t.Error("Failed to return the content unchanged.")
		}
	})
}

func BenchmarkGetEncodings(b *testing.B) {
	// Define test cases with different Content-Encoding header values
	testCases := []struct {
		name   string
		header string
	}{
		{
			name:   "Single encoding",
			header: "gzip",
		},
		{
			name:   "Multiple encodings",
			header: "gzip, deflate",
		},
		{
			name:   "Empty header",
			header: "",
		},
	}

	// Run the benchmark for each test case
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				getEncodings(tc.header)
			}
		})
	}
}
