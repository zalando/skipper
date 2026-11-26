package builtin

import (
	"net/http"

	"github.com/zalando/skipper/filters"
)

type decompressRequest struct{}

// NewDecompressRequest creates a filter specification for the decompressRequest() filter.
// The filter attempts to decompress the request body, if it was compressed
// with any of deflate, gzip, br or zstd.
//
// If decompression is not possible, but the body is compressed, then it indicates it
// with the "filter::decompress::not-possible" key in the state-bag. If the decompression
// was attempted and failed to get initialized, it indicates it in addition with the
// "filter::decompress::error" state-bag key, storing the error. Due to the streaming,
// decompression may fail after all the filters were processed.
//
// The filter does not need any parameters.
func NewDecompressRequest() filters.Spec {
	return decompressRequest{}
}

func (d decompressRequest) Name() string { return filters.DecompressRequestName }

func (d decompressRequest) CreateFilter([]interface{}) (filters.Filter, error) {
	return d, nil
}

func (d decompressRequest) Response(filters.FilterContext) {}

func (d decompressRequest) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	encs := getEncodings(req.Header.Get("Content-Encoding"))
	if len(encs) == 0 {
		return
	}

	if !encodingsSupported(encs) {
		ctx.StateBag()[DecompressionNotPossible] = true
		return
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = -1

	b, err := newDecodedBody(req.Body, encs)
	if err != nil {
		// we may have already sniffed from the request via the gzip.Reader
		req.Body.Close()
		req.Body = http.NoBody

		sb := ctx.StateBag()
		sb[DecompressionNotPossible] = true
		sb[DecompressionError] = err

		ctx.Logger().Errorf("Error while initializing request decompression: %v", err)
		return
	}

	req.Body = b
}
