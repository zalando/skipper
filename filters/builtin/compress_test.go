package builtin

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	maxTestContent = 81 * 8192
	writeLength    = 8192 + 4096
	writeDelay     = 3 * time.Millisecond
)

var testContent []byte

func init() {
	testContent = make([]byte, maxTestContent)
	n, err := rand.Read(testContent)

	if err != nil {
		panic(err)
	}

	if n != len(testContent) {
		panic(errors.New("failed to generate random content"))
	}
}

func setHeaders(to, from http.Header) {
	for k, _ := range to {
		delete(to, k)
	}

	for k, v := range from {
		to[k] = v
	}
}

func decoder(enc string, r io.Reader) io.Reader {
	switch enc {
	case "gzip":
		rr, err := gzip.NewReader(r)
		if err != nil {
			panic(err)
		}

		return rr
	case "deflate":
		return flate.NewReader(r)
	default:
		panic(unsupportedEncoding)
	}
}

func compareBody(r *http.Response, contentLength int) (bool, error) {
	var c io.Reader = r.Body
	enc := r.Header.Get("Content-Encoding")
	if stringsContain(supportedEncodings, enc) {
		c = decoder(enc, r.Body)

		if cls, ok := c.(io.Closer); ok {
			defer cls.Close()
		}
	}

	b, err := ioutil.ReadAll(c)
	if err != nil {
		return false, err
	}

	return bytes.Equal(b, testContent[:contentLength]), nil
}

func benchmarkCompress(b *testing.B, n int64) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			body := ioutil.NopCloser(&io.LimitedReader{rand.New(rand.NewSource(0)), n})
			req := &http.Request{Header: http.Header{"Accept-Encoding": []string{"gzip,deflate"}}}
			rsp := &http.Response{
				Header: http.Header{"Content-Type": []string{"application/octet-stream"}},
				Body:   body}
			ctx := &filtertest.Context{
				FRequest:  req,
				FResponse: rsp}
			s := NewCompress()
			f, _ := s.CreateFilter(nil)
			f.Response(ctx)
			_, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestCompressArgs(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		args     []interface{}
		err      error
		expected []string
	}{{
		"default set of mime types",
		nil,
		nil,
		defaultCompressMIME,
	}, {
		"extended set of mime types",
		[]interface{}{"...", "x/custom-0", "x/custom-1"},
		nil,
		append(defaultCompressMIME, "x/custom-0", "x/custom-1"),
	}, {
		"reset set of mime types",
		[]interface{}{"x/custom-0", "x/custom-1"},
		nil,
		[]string{"x/custom-0", "x/custom-1"},
	}, {
		"invalid parameter",
		[]interface{}{"x/custom-0", "x/custom-1", 3.14},
		filters.ErrInvalidFilterParameters,
		nil,
	}} {
		s := &compress{}
		f, err := s.CreateFilter(ti.args)

		if ti.err != err {
			t.Error(ti.msg, "failed to fail", ti.err, err)
		}

		if ti.err != nil {
			continue
		}

		c := f.(*compress)

		if len(ti.expected) != len(c.mime) {
			t.Error(ti.msg, "invalid length of mime types")
			continue
		}

		for i, m := range ti.expected {
			if c.mime[i] != m {
				t.Error(ti.msg, "invalid mime type", m, c.mime[i])
			}
		}
	}
}

func TestCompress(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		responseHeader http.Header
		contentLength  int
		compressArgs   []interface{}
		acceptEncoding string
		expectedHeader http.Header
	}{{
		"response already encoded",
		http.Header{"Content-Encoding": []string{"x-custom"}},
		3 * 8192,
		nil,
		"gzip,deflate",
		http.Header{"Content-Encoding": []string{"x-custom"}},
	}, {
		"forgives identity in the response",
		http.Header{"Content-Encoding": []string{"identity"}},
		3 * 8192,
		nil,
		"gzip,deflate",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"encoding prohibited by cache control",
		http.Header{"Cache-Control": []string{"x-test,no-transform"}},
		3 * 8192,
		nil,
		"gzip,deflate",
		http.Header{"Cache-Control": []string{"x-test,no-transform"}},
	}, {
		"unsupported content type",
		http.Header{"Content-Type": []string{"x/custom"}},
		3 * 8192,
		nil,
		"gzip,deflate",
		http.Header{"Content-Type": []string{"x/custom"}},
	}, {
		"custom content type",
		http.Header{"Content-Type": []string{"x/custom"}},
		3 * 8192,
		[]interface{}{"x/custom"},
		"gzip,deflate",
		http.Header{
			"Content-Type":     []string{"x/custom"},
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"does not accept encoding",
		http.Header{},
		3 * 8192,
		nil,
		"",
		http.Header{},
	}, {
		"unknown encoding",
		http.Header{},
		3 * 8192,
		nil,
		"x-custom",
		http.Header{},
	}, {
		"gzip",
		http.Header{},
		3 * 8192,
		nil,
		"x-custom,gzip",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"deflate",
		http.Header{},
		3 * 8192,
		nil,
		"x-custom,deflate",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"weighted first",
		http.Header{},
		3 * 8192,
		nil,
		"x-custom; q=0.8, deflate; q=0.6, gzip; q=0.4",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"weighted last",
		http.Header{},
		3 * 8192,
		nil,
		"gzip; q=0.4, x-custom; q=0.8, deflate; q=0.6",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"drops content length",
		http.Header{"Content-Length": []string{strconv.Itoa(3 * 8192)}},
		3 * 8192,
		nil,
		"gzip,deflate",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"encodes large body",
		http.Header{},
		maxTestContent,
		nil,
		"gzip,deflate",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			setHeaders(w.Header(), ti.responseHeader)
			count := 0
			for count < ti.contentLength {
				wl := writeLength
				if count+wl > len(testContent) {
					wl = len(testContent) - count
				}

				w.Write(testContent[count : count+wl])
				count += wl
				time.Sleep(writeDelay)
			}
		}))

		p := proxytest.New(MakeRegistry(), &eskip.Route{
			Filters: []*eskip.Filter{{Name: CompressName, Args: ti.compressArgs}},
			Backend: s.URL})

		req, err := http.NewRequest("GET", p.URL, nil)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		req.Header.Set("Accept-Encoding", ti.acceptEncoding)

		rsp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		defer rsp.Body.Close()

		rsp.Header.Del("Server")
		rsp.Header.Del("X-Powered-By")
		rsp.Header.Del("Date")
		if rsp.Header.Get("Content-Type") == "application/octet-stream" {
			rsp.Header.Del("Content-Type")
		}

		if !compareHeaders(rsp.Header, ti.expectedHeader) {
			printHeader(t, ti.expectedHeader, ti.msg, "invalid header", "expected")
			printHeader(t, rsp.Header, ti.msg, "invalid header", "got")

			t.Error(ti.msg, "invalid header")
			continue
		}

		if ok, err := compareBody(rsp, ti.contentLength); err != nil {
			t.Error(ti.msg, err)
		} else if !ok {
			t.Error(ti.msg, "invalid content")
		}
	}
}

func BenchmarkCompress0(b *testing.B) { benchmarkCompress(b, 0) }
func BenchmarkCompress2(b *testing.B) { benchmarkCompress(b, 100) }
func BenchmarkCompress4(b *testing.B) { benchmarkCompress(b, 10000) }
func BenchmarkCompress6(b *testing.B) { benchmarkCompress(b, 1000000) }
func BenchmarkCompress8(b *testing.B) { benchmarkCompress(b, 100000000) }
