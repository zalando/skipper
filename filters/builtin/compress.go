package builtin

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const bufferSize = 8192

type encoding struct {
	name string
	q    float32 // encoding client priority
	p    int     // encoding server priority
}

type encodings []*encoding

type compress struct {
	mime             []string
	level            int
	encodingPriority map[string]int
}

type CompressOptions struct {
	// Specifies encodings supported for compression, the order defines priority when Accept-Header has equal quality values, see RFC 7231 section 5.3.1
	Encodings []string
}

type encoder interface {
	io.WriteCloser
	Reset(io.Writer)
	Flush() error
}

var (
	supportedEncodings     = []string{"gzip", "deflate", "br"}
	errUnsupportedEncoding = errors.New("unsupported encoding")
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
	brotliPool = &sync.Pool{New: func() any {
		ge, err := newEncoder("br", flate.BestSpeed)
		if err != nil {
			log.Error(err)
		}
		return ge
	}}
	gzipPool = &sync.Pool{New: func() any {
		ge, err := newEncoder("gzip", flate.BestSpeed)
		if err != nil {
			log.Error(err)
		}
		return ge
	}}
	deflatePool = &sync.Pool{New: func() any {
		fe, err := newEncoder("deflate", flate.BestSpeed)
		if err != nil {
			log.Error(err)
		}
		return fe
	}}
)

func (e encodings) Len() int { return len(e) }
func (e encodings) Less(i, j int) bool {
	if e[i].q != e[j].q {
		return e[i].q > e[j].q // higher first
	}
	return e[i].p < e[j].p // smallest first
}
func (e encodings) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

// NewCompress returns a filter specification that is used to compress
// the response content.
//
// Example:
//
//	r: * -> compress() -> "https://www.example.org";
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
//	r: * -> compress("...", "image/tiff") -> "https://www.example.org";
//
// To reset the supported types, e.g. to compress only HTML, the "..." argument
// needs to be omitted:
//
//	r: * -> compress("text/html") -> "https://www.example.org";
//
// It is possible to control the compression level, by setting it as the first
// filter argument, in front of the MIME types. The default compression level is
// best-speed. The possible values are integers between 0 and 11 (inclusive), where
// 0 means no-compression, 1 means best-speed and 11 means best-compression.
// Example:
//
//	r: * -> compress(9, "image/tiff") -> "https://www.example.org";
//
// The filter also checks the incoming request, if it accepts the supported
// encodings, explicitly stated in the Accept-Encoding header. The filter currently
// supports brotli, gzip and deflate. It does not assume that the client accepts any
// encoding if the Accept-Encoding header is not set. It ignores * in the
// Accept-Encoding header.
//
// Supported encodings are prioritized on:
// - quality value if provided by client
// - server side priority (encodingPriority) otherwise
//
// When compressing the response, it updates the response header. It deletes the
// the Content-Length value triggering the proxy to always return the response
// with chunked transfer encoding, sets the Content-Encoding to the selected
// encoding and sets the Vary: Accept-Encoding header, if missing.
//
// The compression happens in a streaming way, using only a small internal buffer.
func NewCompress() filters.Spec {
	c, err := NewCompressWithOptions(CompressOptions{supportedEncodings})
	if err != nil {
		log.Warningf("Failed to create compress filter: %v", err)
	}
	return c
}

func NewCompressWithOptions(options CompressOptions) (filters.Spec, error) {
	m := map[string]int{}
	for i, v := range options.Encodings {
		if !stringsContain(supportedEncodings, v) {
			return nil, errUnsupportedEncoding
		}
		m[v] = i
	}
	return &compress{encodingPriority: m}, nil
}

func (c *compress) Name() string {
	return filters.CompressName
}

func (c *compress) CreateFilter(args []any) (filters.Filter, error) {
	f := &compress{
		mime:             defaultCompressMIME,
		level:            flate.BestSpeed,
		encodingPriority: c.encodingPriority,
	}

	if len(args) == 0 {
		return f, nil
	}

	if lf, ok := args[0].(float64); ok && math.Trunc(lf) == lf {
		f.level = int(lf)
		if f.level < flate.HuffmanOnly || f.level > brotli.BestCompression {
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

	cc := strings.ToLower(r.Header.Get("Cache-Control"))
	if strings.Contains(cc, "no-transform") {
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

func (c *compress) acceptedEncoding(r *http.Request) string {
	var encs encodings

	for s := range splitSeq(r.Header.Get("Accept-Encoding"), ",") {

		name, weight, hasWeight := strings.Cut(s, ";")

		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}

		prio, ok := c.encodingPriority[name]
		if !ok {
			continue
		}

		enc := &encoding{name, 1, prio}
		encs = append(encs, enc)

		if !hasWeight {
			continue
		}

		weight = strings.TrimSpace(weight)
		if !strings.HasPrefix(weight, "q=") {
			continue
		}

		q, err := strconv.ParseFloat(strings.TrimPrefix(weight, "q="), 32)
		if err != nil {
			continue
		}

		if float32(q) < 0 || float32(q) > 1.0 {
			continue
		}

		enc.q = float32(q)

	}

	if len(encs) == 0 {
		return ""
	}

	sort.Sort(encs)
	return encs[0].name
}

// TODO: use [strings.SplitSeq] added in go1.24 once go1.25 is released.
func splitSeq(s string, sep string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for {
			i := strings.Index(s, sep)
			if i < 0 {
				break
			}
			frag := s[:i]
			if !yield(frag) {
				return
			}
			s = s[i+len(sep):]
		}
		yield(s)
	}
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
	panic(errUnsupportedEncoding)
}

func newEncoder(enc string, level int) (encoder, error) {
	switch enc {
	case "br":
		return brotli.NewWriterLevel(nil, level), nil
	case "gzip":
		if level > gzip.BestCompression {
			level = gzip.BestCompression
		}
		return gzip.NewWriterLevel(nil, level)
	case "deflate":
		if level > flate.BestCompression {
			level = flate.BestCompression
		}
		return flate.NewWriter(nil, level)
	default:
		unsupported()
		return nil, nil
	}
}

func encoderPool(enc string) *sync.Pool {
	switch enc {
	case "br":
		return brotliPool
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
			cerr := e.Close()
			if cerr == nil && level == flate.BestSpeed {
				encoderPool(enc).Put(e)
			}
		}

		if err == nil {
			err = io.EOF
		}

		out.CloseWithError(err)
		in.Close()
	}()

	if level == flate.BestSpeed {
		e = encoderPool(enc).Get().(encoder)

		// if the pool.New failed to create an encoder,
		// then we already have logged the error
		if e == nil {
			return
		}
	} else {
		e, err = newEncoder(enc, level)
		if err != nil {
			log.Error(err)
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

	enc := c.acceptedEncoding(ctx.Request())
	if enc == "" {
		return
	}

	responseHeader(rsp, enc)
	responseBody(rsp, enc, c.level)
}
