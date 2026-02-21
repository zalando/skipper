package builtin

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"maps"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"

	"github.com/andybalholm/brotli"
)

const (
	maxTestContent = 81 * 8192
	writeLength    = 8192 + 4096
	writeDelay     = 3 * time.Millisecond
)

type errorReader struct {
	content string
	err     error
}

var testContent []byte

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	testContent = make([]byte, maxTestContent)
	n, err := r.Read(testContent)

	if err != nil {
		panic(err)
	}

	if n != len(testContent) {
		panic(errors.New("failed to generate random content"))
	}
}

func (er *errorReader) Read(b []byte) (int, error) {
	if er.content == "" {
		return 0, er.err
	}

	n := copy(b, er.content)
	er.content = er.content[n:]
	return n, nil
}

func setHeaders(to, from http.Header) {
	for k := range to {
		delete(to, k)
	}

	maps.Copy(to, from)
}

func decoder(enc string, r io.Reader) io.Reader {
	switch enc {
	case "br":
		rr := brotli.NewReader(r)

		return rr
	case "gzip":
		rr, err := gzip.NewReader(r)
		if err != nil {
			panic(err)
		}

		return rr
	case "deflate":
		return flate.NewReader(r)
	default:
		panic(errUnsupportedEncoding)
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

	b, err := io.ReadAll(c)
	if err != nil {
		return false, err
	}

	return bytes.Equal(b, testContent[:contentLength]), nil
}

func benchmarkCompress(b *testing.B, n int64, encoding []string) {
	s := NewCompress()
	f, _ := s.CreateFilter(nil)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			body := io.NopCloser(&io.LimitedReader{R: rand.New(rand.NewSource(0)), N: n})
			req := &http.Request{Header: http.Header{"Accept-Encoding": encoding}}
			rsp := &http.Response{
				Header: http.Header{"Content-Type": []string{"application/octet-stream"}},
				Body:   body}
			ctx := &filtertest.Context{
				FRequest:  req,
				FResponse: rsp}
			f.Response(ctx)
			_, err := io.ReadAll(rsp.Body)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestCompressArgs(t *testing.T) {
	for _, ti := range []struct {
		msg           string
		args          []any
		err           error
		expectedMime  []string
		expectedLevel int
	}{{
		"default set of mime types",
		nil,
		nil,
		defaultCompressMIME,
		flate.BestSpeed,
	}, {
		"extended set of mime types",
		[]any{"...", "x/custom-0", "x/custom-1"},
		nil,
		append(defaultCompressMIME, "x/custom-0", "x/custom-1"),
		flate.BestSpeed,
	}, {
		"reset set of mime types",
		[]any{"x/custom-0", "x/custom-1"},
		nil,
		[]string{"x/custom-0", "x/custom-1"},
		flate.BestSpeed,
	}, {
		"invalid parameter",
		[]any{"x/custom-0", "x/custom-1", 3.14},
		filters.ErrInvalidFilterParameters,
		nil,
		flate.BestSpeed,
	}, {
		"non integer level",
		[]any{3.14, "...", "x/custom"},
		filters.ErrInvalidFilterParameters,
		nil,
		0,
	}, {
		"level too small",
		[]any{-1, "...", "x/custom"},
		filters.ErrInvalidFilterParameters,
		nil,
		0,
	}, {
		"level too big",
		[]any{10, "...", "x/custom"},
		filters.ErrInvalidFilterParameters,
		nil,
		0,
	}, {
		"set level only",
		[]any{float64(6)},
		nil,
		defaultCompressMIME,
		6,
	}, {
		"set level and reset mime",
		[]any{float64(6), "x/custom-0", "x/custom-1"},
		nil,
		[]string{"x/custom-0", "x/custom-1"},
		6,
	}, {
		"set level and extend mime",
		[]any{float64(6), "...", "x/custom-0", "x/custom-1"},
		nil,
		append(defaultCompressMIME, "x/custom-0", "x/custom-1"),
		6,
	}} {
		s := NewCompress()
		f, err := s.CreateFilter(ti.args)

		if ti.err != err {
			t.Error(ti.msg, "unexpected error value", ti.err, err)
		}

		if err != nil {
			continue
		}

		c := f.(*compress)

		if len(ti.expectedMime) != len(c.mime) {
			t.Error(ti.msg, "invalid length of mime types")
			continue
		}

		for i, m := range ti.expectedMime {
			if c.mime[i] != m {
				t.Error(ti.msg, "invalid mime type", m, c.mime[i])
			}
		}

		if c.level != ti.expectedLevel {
			t.Error(ti.msg, "invalid level", ti.expectedLevel, c.level)
		}
	}
}

func TestCompress(t *testing.T) {
	for _, ti := range []struct {
		msg            string
		responseHeader http.Header
		contentLength  int
		compressArgs   []any
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
		[]any{"x/custom"},
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
		"gzip, no compression",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.NoCompression)},
		"x-custom,gzip",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"gzip, best speed",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.BestSpeed)},
		"x-custom,gzip",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"gzip, stdlib default",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.DefaultCompression)},
		"x-custom,gzip",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"gzip, best compression",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.BestCompression)},
		"x-custom,gzip",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"gzip, higher compression",
		http.Header{},
		3 * 8192,
		[]any{float64(brotli.BestCompression)},
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
		"deflate, no compression",
		http.Header{},
		3 * 8192,
		[]any{float64(flate.NoCompression)},
		"x-custom,deflate",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"deflate, best speed",
		http.Header{},
		3 * 8192,
		[]any{float64(flate.BestSpeed)},
		"x-custom,deflate",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"deflate",
		http.Header{},
		3 * 8192,
		[]any{float64(flate.DefaultCompression)},
		"x-custom,deflate",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"deflate",
		http.Header{},
		3 * 8192,
		[]any{float64(flate.BestCompression)},
		"x-custom,deflate",
		http.Header{
			"Content-Encoding": []string{"deflate"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"brotli",
		http.Header{},
		3 * 8192,
		nil,
		"x-custom,br",
		http.Header{
			"Content-Encoding": []string{"br"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"brotli, best speed",
		http.Header{},
		3 * 8192,
		[]any{float64(brotli.BestSpeed)},
		"x-custom,br",
		http.Header{
			"Content-Encoding": []string{"br"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"brotli, stdlib default",
		http.Header{},
		3 * 8192,
		[]any{float64(brotli.DefaultCompression)},
		"x-custom,br",
		http.Header{
			"Content-Encoding": []string{"br"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"brotli, best compression",
		http.Header{},
		3 * 8192,
		[]any{float64(brotli.BestCompression)},
		"x-custom,br",
		http.Header{
			"Content-Encoding": []string{"br"},
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
	}, {
		"multiple compression, priority to gzip",
		http.Header{},
		3 * 8192,
		[]any{float64(brotli.BestCompression)},
		"x-custom,br,gzip,deflate",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"multiple compression, priority to gzip",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.BestCompression)},
		"x-custom,gzip,deflate",
		http.Header{
			"Content-Encoding": []string{"gzip"},
			"Vary":             []string{"Accept-Encoding"}},
	}, {
		"malformed accept encoding",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.BestCompression)},
		"x-custom, ;q=3",
		http.Header{},
	}, {
		"invalid q value",
		http.Header{},
		3 * 8192,
		[]any{float64(gzip.BestCompression)},
		"x-custom;q=1.1",
		http.Header{},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				setHeaders(w.Header(), ti.responseHeader)
				if _, ok := ti.responseHeader["Content-Type"]; !ok {
					w.Header().Set("Content-Type", "application/octet-stream")
				}

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
			defer s.Close()

			p := proxytest.New(MakeRegistry(), &eskip.Route{
				Filters: []*eskip.Filter{{Name: filters.CompressName, Args: ti.compressArgs}},
				Backend: s.URL})
			defer p.Close()

			req, err := http.NewRequest("GET", p.URL, nil)
			if err != nil {
				t.Error(err)
				return
			}

			req.Header.Set("Accept-Encoding", ti.acceptEncoding)

			rsp, err := http.DefaultTransport.RoundTrip(req)
			if err != nil {
				t.Error(err)
				return
			}

			defer rsp.Body.Close()

			rsp.Header.Del("Server")
			rsp.Header.Del("X-Powered-By")
			rsp.Header.Del("Date")
			if rsp.Header.Get("Content-Type") == "application/octet-stream" {
				rsp.Header.Del("Content-Type")
			}

			assert.Equal(t, ti.expectedHeader, rsp.Header)

			if ok, err := compareBody(rsp, ti.contentLength); err != nil {
				t.Error(err)
			} else if !ok {
				t.Error("invalid content")
			}
		})
	}
}

func TestForwardError(t *testing.T) {
	spec := NewCompress()
	f, err := spec.CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	testError := errors.New("test error")
	req := &http.Request{Header: http.Header{"Accept-Encoding": []string{"gzip,deflate"}}}
	rsp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   io.NopCloser(&errorReader{"test-content", testError})}
	ctx := &filtertest.Context{FRequest: req, FResponse: rsp}
	f.Response(ctx)
	enc := rsp.Header.Get("Content-Encoding")
	dec := decoder(enc, rsp.Body)
	b, err := io.ReadAll(dec)
	if string(b) != "test-content" || err != testError {
		t.Error("failed to forward error", string(b), err)
	}
}

type readCloser struct {
	io.Reader
	done chan struct{}
}

func (h *readCloser) Close() error {
	close(h.done)
	return nil
}

func TestCompressWithEncodings(t *testing.T) {
	spec, err := NewCompressWithOptions(CompressOptions{Encodings: []string{"br", "gzip"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := spec.CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	req := &http.Request{Header: http.Header{"Accept-Encoding": []string{"gzip,br,deflate"}}}
	body := &readCloser{Reader: &io.LimitedReader{R: rand.New(rand.NewSource(0)), N: 100}, done: done}
	rsp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body:   body,
	}

	ctx := &filtertest.Context{FRequest: req, FResponse: rsp}
	f.Response(ctx)

	io.Copy(io.Discard, rsp.Body)
	<-done

	enc := rsp.Header.Get("Content-Encoding")
	if enc != "br" {
		t.Error("unexpected value", enc)
	}
}

func TestCompressWithUnsupportedEncodings(t *testing.T) {
	_, err := NewCompressWithOptions(CompressOptions{Encodings: []string{"br", "notSupported", "gzip"}})
	if err == nil {
		t.Error("expect error")
	}
}

func TestStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	randReader := rand.New(rand.NewSource(0))
	writeRandomN := func(w io.Writer, n int64) error {
		nw, err := io.CopyN(w, randReader, n)
		if nw != n {
			return errors.New("failed to write random bytes")
		}

		return err
	}

	timeoutCall := func(to time.Duration, call func(c chan<- error)) error {
		c := make(chan error)
		go call(c)

		select {
		case err := <-c:
			return err
		case <-time.After(to):
			return errors.New("timeout")
		}
	}

	chunkDelay := 120 * time.Millisecond

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")

		err := writeRandomN(w, 1<<14)
		if err != nil {
			t.Error(err)
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		time.Sleep(chunkDelay)
		err = writeRandomN(w, 1<<14)
		if err != nil {
			t.Error(err)
		}
	}))
	defer s.Close()

	p := proxytest.New(MakeRegistry(), &eskip.Route{
		Filters: []*eskip.Filter{{Name: filters.CompressName}},
		Backend: s.URL})
	defer p.Close()

	var body io.ReadCloser
	if err := timeoutCall(chunkDelay/2, func(c chan<- error) {
		rsp, err := http.Get(p.URL)
		if err != nil {
			c <- err
			return
		}

		// this body is closed in the enclosing function
		body = rsp.Body

		if rsp.StatusCode != http.StatusOK {
			c <- errors.New("failed to make request")
			return
		}

		const preread = 1 << 6
		n, err := body.Read(make([]byte, preread))
		if err != nil {
			c <- err
			return
		}

		if n != preread {
			c <- errors.New("failed to preread from the body")
			return
		}

		c <- nil
	}); err != nil {
		t.Error(err)
		return
	}

	defer body.Close()

	if err := timeoutCall(chunkDelay*3/2, func(c chan<- error) {
		_, err := io.ReadAll(body)
		c <- err
	}); err != nil {
		t.Error(err)
	}
}

func TestPoolRelease(t *testing.T) {
	// This test needs can reproduce a bug caused by the wrong order of closing the encoders and putting
	// them back to the pool.
	//
	// https://github.com/zalando/skipper/issues/1312
	//
	// Enable it only for long running tests.
	t.Skip()

	const (
		numberOfTries = 10000
		concurrency   = 256
	)

	f, err := NewCompress().CreateFilter(nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			for range numberOfTries {
				ctx := &filtertest.Context{
					FRequest: &http.Request{
						Header: http.Header{
							"Accept-Encoding": []string{"gzip"},
						},
					},
					FResponse: &http.Response{
						Header: http.Header{
							"Content-Length": []string{"9000"},
							"Content-Type":   []string{"application/octet-stream"},
						},
						Body: io.NopCloser(bytes.NewBuffer(testContent[:9000])),
					},
				}

				f.Response(ctx)
				io.ReadAll(ctx.Response().Body)
				ctx.Response().Body.Close()
			}

		})
	}

	wg.Wait()
}

func BenchmarkCompressGzip0(b *testing.B) { benchmarkCompress(b, 0, []string{"gzip,deflate"}) }
func BenchmarkCompressGzip2(b *testing.B) { benchmarkCompress(b, 100, []string{"gzip,deflate"}) }
func BenchmarkCompressGzip4(b *testing.B) { benchmarkCompress(b, 10000, []string{"gzip,deflate"}) }
func BenchmarkCompressGzip6(b *testing.B) { benchmarkCompress(b, 1000000, []string{"gzip,deflate"}) }
func BenchmarkCompressGzip8(b *testing.B) { benchmarkCompress(b, 100000000, []string{"gzip,deflate"}) }

func BenchmarkCompressBrotli0(b *testing.B) { benchmarkCompress(b, 0, []string{"br"}) }
func BenchmarkCompressBrotli2(b *testing.B) { benchmarkCompress(b, 100, []string{"br"}) }
func BenchmarkCompressBrotli4(b *testing.B) { benchmarkCompress(b, 10000, []string{"br"}) }
func BenchmarkCompressBrotli6(b *testing.B) { benchmarkCompress(b, 1000000, []string{"br"}) }
func BenchmarkCompressBrotli8(b *testing.B) { benchmarkCompress(b, 100000000, []string{"br"}) }

func BenchmarkCanEncodeEntity(b *testing.B) {
	testCases := []struct {
		name string
		resp *http.Response
		mime []string
	}{
		{
			name: "Valid Content-Encoding and MIME",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"x-test"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Unsupported Content-Encoding",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"gzip"},
					"Cache-Control":    []string{"x-test"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Empty Content-Encoding",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{""},
					"Cache-Control":    []string{"x-test"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Multiple Cache-Control without No-Transform",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"x-test-1", "x-test"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "No-Transform Cache-Control",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"no-transform"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Multiple Cache-Control with No-Transform",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"x-test", "no-transform"},
					"Content-Type":     []string{"x/custom"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Content-Type with Boundary",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"x-test"},
					"Content-Type":     []string{"x/custom; boundary=12345"},
				},
			},
			mime: []string{"x/custom"},
		},
		{
			name: "Unsupported MIME Type",
			resp: &http.Response{
				Header: http.Header{
					"Content-Encoding": []string{"identity"},
					"Cache-Control":    []string{"x-test"},
					"Content-Type":     []string{"x/custom-unsupported"},
				},
			},
			mime: []string{"x/custom"},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				canEncodeEntity(tc.resp, tc.mime)
			}
		})
	}
}

func BenchmarkAcceptedEncoding(b *testing.B) {
	testCases := []struct {
		name           string
		acceptEncoding string
	}{
		{
			name:           "No Accept-Encoding header",
			acceptEncoding: "",
		},
		{
			name:           "Single encoding - gzip",
			acceptEncoding: "gzip",
		},
		{
			name:           "Multiple Encodings with no q value",
			acceptEncoding: "gzip, deflate",
		},
		{
			name:           "Unsupported encoding",
			acceptEncoding: "x-custom",
		},
		{
			name:           "Multiple encodings with priorities",
			acceptEncoding: "gzip;q=0.8, deflate;q=0.9, br;q=1.0",
		},
		{
			name:           "Weighted encoding with default priority",
			acceptEncoding: "gzip;q=0.5, deflate;q=0.7",
		},
		{
			name:           "Multiple encodings without prefix 'q='",
			acceptEncoding: "gzip;q, deflate; ",
		},
		{
			name:           "Encoding with wrong q value",
			acceptEncoding: "gzip;q=1.2",
		},
	}

	c := &compress{
		encodingPriority: map[string]int{
			"gzip":    1,
			"deflate": 2,
			"br":      0,
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			req := &http.Request{
				Header: http.Header{
					"Accept-Encoding": []string{tc.acceptEncoding},
				},
			}

			for i := 0; i < b.N; i++ {
				c.acceptedEncoding(req)
			}
		})
	}
}
