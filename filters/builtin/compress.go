package builtin

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

var (
	supportedEncodings  = []string{"gzip", "deflate"}
	unsupportedEncoding = errors.New("unsupported encoding")
)

var defaultCompressMIME = []string{"text/plain", "text/html", "application/json"}

func (e encodings) Len() int           { return len(e) }
func (e encodings) Less(i, j int) bool { return e[i].q > e[j].q } // higher first
func (e encodings) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }

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
	log.Info("header", r.Header.Get("Accept-Encoding"))
	for _, s := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		log.Info("checking", s)
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

func encoder(enc string, w io.Writer) io.WriteCloser {
	switch enc {
	case "gzip":
		return gzip.NewWriter(w)
	case "deflate":
		w, err := flate.NewWriter(w, flate.DefaultCompression)
		if err != nil {
			// too late to return error, the compress/flate doc states, that it returns an error
			// only if the compression level is invalid. Considered as an implemenation error.
			panic(err)
		}

		return w
	default:
		// caller code in the package cannot call this with unsupported
		// encoding name. Considered as an implemenation error.
		panic(unsupportedEncoding)
	}
}

func encode(out *PipedBody, in io.ReadCloser, enc string) {
	e := encoder(enc, out)
	b := make([]byte, bufferSize)
	_, err := io.CopyBuffer(e, in, b)
	if err != nil {
		log.Error(err)
	}

	e.Close()
	out.WriteError(io.EOF)
	in.Close()
}

func responseBody(r *http.Response, enc string) {
	in := r.Body
	out := NewPipedBody()
	r.Body = out
	go encode(out, in, enc)
}

func (c *compress) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()
	if !canEncodeEntity(rsp, c.mime) {
		return
	}

	req := ctx.Request()
	enc := acceptedEncoding(req)
	if enc == "" {
		log.Info("does not accept encoding")
		return
	}

	responseHeader(rsp, enc)
	responseBody(rsp, enc)
}
