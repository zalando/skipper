package builtin

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"

	"github.com/zalando/skipper/filters"
)

const (
	// DecompressionNotPossible is the state-bag key to indicate
	// to the subsequent filters during response processing that the
	// content is compressed, but decompression was not possible, e.g
	// because the encoding is not supported.
	DecompressionNotPossible = "filter::decompress::not-possible"

	// DecompressionError is the state-bag key to indicate to the
	// subsequent filters during response processing that the
	// decompression of the content was attempted but failed. The
	// response body may have been sniffed, and therefore it was
	// discarded.
	DecompressionError = "filter::decompress::error"
)

type decodedBody struct {
	enc        string
	original   io.Closer
	decoder    io.ReadCloser
	isFromPool bool
}

type decodingError struct {
	decoder  error
	original error
}

type decompress struct{}

// workaround to make brotli library compatible with decompress
type brotliWrapper struct {
	brotli.Reader
}

func (brotliWrapper) Close() error { return nil }

var supportedEncodingsDecompress = map[string]*sync.Pool{
	"gzip":    {},
	"deflate": {},
	"br":      {},
}

func init() {
	// #cpu * 4: pool size decided based on some
	// simple tests, checking performance by binary
	// steps (https://github.com/zalando/skipper)
	for enc, pool := range supportedEncodingsDecompress {
		for i := 0; i < runtime.NumCPU()*4; i++ {
			pool.Put(newDecoder(enc))
		}
	}
}

func newDecoder(enc string) io.ReadCloser {
	switch enc {
	case "gzip":
		return new(gzip.Reader)
	case "br":
		return new(brotliWrapper)
	default:
		return flate.NewReader(nil)
	}
}

func fromPool(enc string) (io.ReadCloser, bool) {
	d, ok := supportedEncodingsDecompress[enc].Get().(io.ReadCloser)
	return d, ok
}

func reset(decoder, original io.ReadCloser, enc string) error {
	switch enc {
	case "gzip":
		return decoder.(*gzip.Reader).Reset(original)
	case "br":
		return decoder.(*brotliWrapper).Reset(original)
	default:
		return decoder.(flate.Resetter).Reset(original, nil)
	}
}

func newDecodedBody(original io.ReadCloser, encs []string) (body io.ReadCloser, err error) {
	if len(encs) == 0 {
		body = original
		return
	}

	last := len(encs) - 1
	enc := encs[last]
	encs = encs[:last]

	decoder, isFromPool := fromPool(enc)
	if !isFromPool {
		decoder = newDecoder(enc)
	}

	if err = reset(decoder, original, enc); err != nil {
		return
	}

	decoded := decodedBody{
		enc:        enc,
		original:   original,
		decoder:    decoder,
		isFromPool: isFromPool,
	}

	return newDecodedBody(decoded, encs)
}

func (b decodedBody) Read(p []byte) (int, error) {
	return b.decoder.Read(p)
}

func (b decodedBody) Close() error {
	derr := b.decoder.Close()
	if b.isFromPool {
		if derr != nil {
			supportedEncodingsDecompress[b.enc].Put(newDecoder(b.enc))
		} else {
			supportedEncodingsDecompress[b.enc].Put(b.decoder)
		}
	}

	oerr := b.original.Close()
	var err error
	if derr != nil || oerr != nil {
		err = decodingError{
			decoder:  derr,
			original: oerr,
		}
	}

	return err
}

func (e decodingError) Error() string {
	switch {
	case e.decoder == nil:
		return e.original.Error()
	case e.original == nil:
		return e.decoder.Error()
	default:
		return fmt.Sprintf("%v; %v", e.decoder, e.original)
	}
}

// NewDecompress creates a filter specification for the decompress() filter.
// The filter attempts to decompress the response body, if it was compressed
// with any of deflate, gzip or br.
//
// If decompression is not possible, but the body is compressed, then it indicates it
// with the "filter::decompress::not-possible" key in the state-bag. If the decompression
// was attempted and failed to get initialized, it indicates it in addition with the
// "filter::decompress::error" state-bag key, storing the error. Due to the streaming,
// decompression may fail after all the filters were processed.
//
// The filter does not need any parameters.
func NewDecompress() filters.Spec {
	return decompress{}
}

func (d decompress) Name() string { return filters.DecompressName }

func (d decompress) CreateFilter([]any) (filters.Filter, error) {
	return d, nil
}

func (d decompress) Request(filters.FilterContext) {}

func getEncodings(header string) []string {
	var encs []string
	for r := range splitSeq(header, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			encs = append(encs, r)
		}
	}

	return encs
}

func encodingsSupported(encs []string) bool {
	for _, e := range encs {
		if _, supported := supportedEncodingsDecompress[e]; !supported {
			return false
		}
	}

	return true
}

func (d decompress) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()

	encs := getEncodings(rsp.Header.Get("Content-Encoding"))
	if len(encs) == 0 {
		return
	}

	if !encodingsSupported(encs) {
		ctx.StateBag()[DecompressionNotPossible] = true
		return
	}

	rsp.Header.Del("Content-Encoding")
	rsp.Header.Del("Vary")
	rsp.Header.Del("Content-Length")
	rsp.ContentLength = -1

	b, err := newDecodedBody(rsp.Body, encs)
	if err != nil {
		// we may have already sniffed from the response via the gzip.Reader
		rsp.Body.Close()
		rsp.Body = http.NoBody

		sb := ctx.StateBag()
		sb[DecompressionNotPossible] = true
		sb[DecompressionError] = err

		ctx.Logger().Errorf("Error while initializing decompression: %v", err)
		return
	}

	rsp.Body = b
}
