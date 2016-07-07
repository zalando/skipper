package builtin

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/zalando/skipper/filters"
)

const bufferSize = 8192

type encoding struct {
	name string
	q    float32
}

type encodings []*encoding

type compress struct {
	mime  []string
	level int
}

type encoder interface {
	io.WriteCloser
	Reset(io.Writer)
	Flush() error
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
		ge, err := newEncoder("gzip", flate.BestSpeed)
		if err != nil {
			panic(err)
		}

		gzipPool.Put(ge)

		fe, err := newEncoder("deflate", flate.BestSpeed)
		if err != nil {
			panic(err)
		}

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
// It is possible to control the compression level, by setting it as the first
// filter argument, in front of the MIME types. The default compression level is
// best-speed. The possible values are integers between 0 and 9 (inclusive), where
// 0 means no-compression, 1 means best-speed and 9 means best-compression.
// Example:
//
// 	* -> compress(9, "image/tiff") -> "https://www.example.org"
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
	f := &compress{
		mime:  defaultCompressMIME,
		level: flate.BestSpeed}

	if len(args) == 0 {
		return f, nil
	}

	if lf, ok := args[0].(float64); ok && math.Trunc(lf) == lf {
		f.level = int(lf)
		_, err := gzip.NewWriterLevel(nil, f.level)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}

		_, err = flate.NewWriter(nil, f.level)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}

		args = args[1:]
	}

	if len(args) == 0 {
		return f, nil
	}

	if args[0] == "..." {
		args = args[1:]
	} else {
		f.mime = nil
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

func newEncoder(enc string, level int) (encoder, error) {
	switch enc {
	case "gzip":
		return gzip.NewWriterLevel(nil, level)
	case "deflate":
		return flate.NewWriter(nil, level)
	default:
		unsupported()
		return nil, nil
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

func encode(out *io.PipeWriter, in io.ReadCloser, enc string, level int) {
	var (
		e   encoder
		err error
	)

	defer func() {
		if e != nil {
			e.Close()
		}

		if err == nil {
			err = io.EOF
		}

		out.CloseWithError(err)
		in.Close()
	}()

	if level == flate.BestSpeed {
		pool := encoderPool(enc)
		pe := pool.Get()
		if pe != nil {
			e = pe.(encoder)
			defer pool.Put(pe)
		}
	}

	if e == nil {
		e, err = newEncoder(enc, level)
		if err != nil {
			return
		}
	}

	e.Reset(out)

	b := make([]byte, bufferSize)
	for {
		n, rerr := in.Read(b)
		if n > 0 {
			_, err = e.Write(b[:n])
			if err != nil {
				break
			}

			err = e.Flush()
			if err != nil {
				break
			}
		}

		if rerr != nil {
			err = rerr
			break
		}
	}
}

func responseBody(rsp *http.Response, enc string, level int) {
	in := rsp.Body
	r, w := io.Pipe()
	rsp.Body = r
	go encode(w, in, enc, level)
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
	responseBody(rsp, enc, c.level)
}
