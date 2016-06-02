package builtin

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const bufferSize = 8192

type encoding struct {
	name string
	q    float32
}

type encodings []*encoding

type compress struct {
	mime []string
}

type encoder interface {
	io.WriteCloser
	Reset(io.Writer)
}

var (
	supportedEncodings  = []string{"gzip", "deflate"}
	unsupportedEncoding = errors.New("unsupported encoding")
)

var defaultCompressMIME = []string{
	"text/plain",
	"text/html",
	"application/json",
	"application/javascript",
	"application/x-javascript",
	"text/javascript",
	"text/css",
	"image/svg+xml",
	"application/octet-stream",
}

var (
	gzipPool    = &sync.Pool{}
	deflatePool = &sync.Pool{}
)

func init() {
	// #cpu * 4: pool size decided based on some
	// simple tests, checking performance by binary
	// steps
	for i := 0; i < runtime.NumCPU()*4; i++ {
		ge := newEncoder("gzip")
		gzipPool.Put(ge)
		fe := newEncoder("deflate")
		deflatePool.Put(fe)
	}
}

func (e encodings) Len() int           { return len(e) }
func (e encodings) Less(i, j int) bool { return e[i].q > e[j].q } // higher first
func (e encodings) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }

// Returns a filter specification that is used to compress the response content.
//
// Example:
//
// 	* -> compress() -> "https://www.example.org"
//
// The filter, when executed on the response path, checks if the response
// entity can be compressed. To decide, it checks the Content-Encoding, the
// Cache-Control and the Content-Type headers. It doesn't compress the content
// if the Content-Encoding is set to other than identity, or the Cache-Control
// applies the no-transform pragma, or the Content-Type is set to an unsupported
// value.
//
// The default supported content types are: text/plain, text/html,
// application/json, application/javascript, application/x-javascript,
// text/javascript, text/css, image/svg+xml, application/octet-stream.
//
// The default set of MIME types can be reset or extended by passing in the desired
// types as filter arguments. When extending the defaults, the first argument needs
// to be "...". E.g. to compress tiff in addition to the defaults:
//
// 	* -> compress("...", "image/tiff") -> "https://www.example.org"
//
// To reset the supported types, e.g. to compress only HTML, the "..." argument
// needs to be omitted:
//
// 	* -> compress("text/html") -> "https://www.example.org"
//
// The filter also checks the incoming request, if it accepts the supported
// encodings, explicitly stated in the Accept-Encoding header. The filter currently
// supports gzip and deflate. It does not assume that the client accepts any
// encoding if the Accept-Encoding header is not set. It ignores * in the
// Accept-Encoding header.
//
// When compressing the response, it updates the response header. It deletes the
// the Content-Length value triggering the proxy to always return the response
// with chunked transfer encoding, sets the Content-Encoding to the selected
// encoding and sets the Vary: Accept-Encoding header, if missing.
//
// The compression happens in a streaming way, using only a small internal buffer.
//
func NewCompress() filters.Spec { return &compress{} }

func (c *compress) Name() string {
	return CompressName
}

func (c *compress) CreateFilter(args []interface{}) (filters.Filter, error) {
	f := &compress{}

	if len(args) == 0 {
		f.mime = defaultCompressMIME
		return f, nil
	}

	if args[0] == "..." {
		f.mime = defaultCompressMIME
		args = args[1:]
	}

	for _, a := range args {
		if s, ok := a.(string); ok {
			f.mime = append(f.mime, s)
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return f, nil
}

func (c *compress) Request(_ filters.FilterContext) {}

func stringsContain(ss []string, s string, transform ...func(string) string) bool {
	for _, si := range ss {
		for _, t := range transform {
			si = t(si)
		}

		if si == s {
			return true
		}
	}

	return false
}

func canEncodeEntity(r *http.Response, mime []string) bool {
	if ce := r.Header.Get("Content-Encoding"); ce != "" && ce != "identity" /* forgiving for identity */ {
		return false
	}

	cc := strings.Split(r.Header.Get("Cache-Control"), ",")
	if stringsContain(cc, "no-transform", strings.TrimSpace, strings.ToLower) {
		return false
	}

	ct := r.Header.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}

	if !stringsContain(mime, ct) {
		return false
	}

	return true
}

func acceptedEncoding(r *http.Request) string {
	var encs encodings
	for _, s := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		sp := strings.Split(s, ";")
		if len(sp) == 0 {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(sp[0]))
		if !stringsContain(supportedEncodings, name) {
			continue
		}

		enc := &encoding{name, 1}
		encs = append(encs, enc)

		for _, spi := range sp[1:] {
			spi = strings.TrimSpace(spi)
			if !strings.HasPrefix(spi, "q=") {
				continue
			}

			q, err := strconv.ParseFloat(strings.TrimPrefix(spi, "q="), 32)
			if err != nil {
				continue
			}

			enc.q = float32(q)
			break
		}
	}

	if len(encs) == 0 {
		return ""
	}

	sort.Sort(encs)
	return encs[0].name
}

func responseHeader(r *http.Response, enc string) {
	r.Header.Del("Content-Length")
	r.Header.Set("Content-Encoding", enc)

	if !stringsContain(r.Header["Vary"], "Accept-Encoding", http.CanonicalHeaderKey) {
		r.Header.Add("Vary", "Accept-Encoding")
	}
}

// Not handled encoding is considered as an implementation error, since
// these functions are only called from inside the package, and the
// encoding should be selected from a predefined set.
func unsupported() {
	panic(unsupportedEncoding)
}

func newEncoder(enc string) encoder {
	switch enc {
	case "gzip":
		gw, err := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		if err != nil {
			// This is considered as an implementation error, since the compress/gzip doc
			// states that it returns an error only if the compression level is invalid.
			panic(err)
		}

		return gw
	case "deflate":
		fw, err := flate.NewWriter(nil, flate.BestSpeed)
		if err != nil {
			// This is considered as an implementation error, since the compress/flate doc
			// states that it returns an error only if the compression level is invalid.
			panic(err)
		}

		return fw
	default:
		unsupported()
		return nil
	}
}

func encoderPool(enc string) *sync.Pool {
	switch enc {
	case "gzip":
		return gzipPool
	case "deflate":
		return deflatePool
	default:
		unsupported()
		return nil
	}
}

func encode(out *io.PipeWriter, in io.ReadCloser, enc string) {
	pool := encoderPool(enc)
	pe := pool.Get()
	var e encoder
	if pe == nil {
		e = newEncoder(enc)
	} else {
		e = pe.(encoder)
		defer pool.Put(pe)
	}

	e.Reset(out)

	b := make([]byte, bufferSize)
	_, err := io.CopyBuffer(e, in, b)
	if err != nil {
		log.Error(err)
	}

	if err == nil {
		err = io.EOF
	}

	e.Close()
	out.CloseWithError(err)
	in.Close()
}

func responseBody(rsp *http.Response, enc string) {
	in := rsp.Body
	r, w := io.Pipe()
	rsp.Body = r
	go encode(w, in, enc)
}

func (c *compress) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()
	if !canEncodeEntity(rsp, c.mime) {
		return
	}

	enc := acceptedEncoding(ctx.Request())
	if enc == "" {
		return
	}

	responseHeader(rsp, enc)
	responseBody(rsp, enc)
}
