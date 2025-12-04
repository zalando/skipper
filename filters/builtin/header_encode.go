package builtin

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	xencoding "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

type encodeTyp int

const (
	requestEncoder encodeTyp = iota + 1
	responseEncoder
)

type encodeHeaderSpec struct {
	typ encodeTyp
}

type encodeHeader struct {
	typ     encodeTyp
	header  string
	encoder *xencoding.Encoder
}

func NewEncodeRequestHeader() *encodeHeaderSpec {
	return &encodeHeaderSpec{
		typ: requestEncoder,
	}
}
func NewEncodeResponseHeader() *encodeHeaderSpec {
	return &encodeHeaderSpec{
		typ: responseEncoder,
	}
}

func (spec *encodeHeaderSpec) Name() string {
	switch spec.typ {
	case requestEncoder:
		return filters.EncodeRequestHeaderName
	case responseEncoder:
		return filters.EncodeResponseHeaderName
	}
	return "unknown"
}

func (spec *encodeHeaderSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	header, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	to, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	var (
		encoder *xencoding.Encoder
	)

	switch to {
	case "ISO8859_1":
		encoder = charmap.ISO8859_1.NewEncoder()
	case "ISO8859_10":
		encoder = charmap.ISO8859_10.NewEncoder()
	case "ISO8859_13":
		encoder = charmap.ISO8859_13.NewEncoder()
	case "ISO8859_14":
		encoder = charmap.ISO8859_14.NewEncoder()
	case "ISO8859_15":
		encoder = charmap.ISO8859_15.NewEncoder()
	case "ISO8859_16":
		encoder = charmap.ISO8859_16.NewEncoder()
	case "ISO8859_2":
		encoder = charmap.ISO8859_2.NewEncoder()
	case "ISO8859_3":
		encoder = charmap.ISO8859_3.NewEncoder()
	case "ISO8859_4":
		encoder = charmap.ISO8859_4.NewEncoder()
	case "ISO8859_5":
		encoder = charmap.ISO8859_5.NewEncoder()
	case "ISO8859_6":
		encoder = charmap.ISO8859_6.NewEncoder()
	case "ISO8859_7":
		encoder = charmap.ISO8859_7.NewEncoder()
	case "ISO8859_8":
		encoder = charmap.ISO8859_8.NewEncoder()
	case "ISO8859_9":
		encoder = charmap.ISO8859_9.NewEncoder()
	case "KOI8R":
		encoder = charmap.KOI8R.NewEncoder()
	case "KOI8U":
		encoder = charmap.KOI8U.NewEncoder()
	case "Macintosh":
		encoder = charmap.Macintosh.NewEncoder()
	case "MacintoshCyrillic":
		encoder = charmap.MacintoshCyrillic.NewEncoder()
	case "Windows1250":
		encoder = charmap.Windows1250.NewEncoder()
	case "Windows1251":
		encoder = charmap.Windows1251.NewEncoder()
	case "Windows1252":
		encoder = charmap.Windows1252.NewEncoder()
	case "Windows1253":
		encoder = charmap.Windows1253.NewEncoder()
	case "Windows1254":
		encoder = charmap.Windows1254.NewEncoder()
	case "Windows1255":
		encoder = charmap.Windows1255.NewEncoder()
	case "Windows1256":
		encoder = charmap.Windows1256.NewEncoder()
	case "Windows1257":
		encoder = charmap.Windows1257.NewEncoder()
	case "Windows1258":
		encoder = charmap.Windows1258.NewEncoder()
	case "Windows874":
		encoder = charmap.Windows874.NewEncoder()
	default:
		log.Errorf("Unknown encoder for %q", to)
		return nil, filters.ErrInvalidFilterParameters
	}

	return &encodeHeader{
		typ:     spec.typ,
		header:  header,
		encoder: encoder,
	}, nil
}

func (f *encodeHeader) Request(ctx filters.FilterContext) {
	if f.typ != requestEncoder {
		return
	}

	s := ctx.Request().Header.Get(f.header)
	if s == "" {
		return
	}

	sNew, err := f.encoder.String(s)
	if err != nil {
		log.Errorf("Failed to encode header value of %q: %v", f.header, err)
	}
	ctx.Request().Header.Set(f.header, sNew)
}

func (f *encodeHeader) Response(ctx filters.FilterContext) {
	if f.typ != responseEncoder {
		return
	}
	s := ctx.Response().Header.Get(f.header)
	if s == "" {
		return
	}

	sNew, err := f.encoder.String(s)
	if err != nil {
		log.Errorf("Failed to encode header value of %q: %v", f.header, err)
	}
	ctx.Response().Header.Set(f.header, sNew)
}
