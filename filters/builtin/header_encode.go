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
	case "Windows1252":
		encoder = charmap.Windows1252.NewEncoder()
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
		log.Errorf("Failed to encode %q: %v", s, err)
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
		log.Errorf("Failed to encode %q: %v", s, err)
	}
	ctx.Response().Header.Set(f.header, sNew)

}
