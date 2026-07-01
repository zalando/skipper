package builtin

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/zalando/skipper/filters"
)

type encodeBase64Type int

const (
	encodeRequestHeaderBase64 encodeBase64Type = iota
	encodeResponseHeaderBase64
)

// encodeBase64 filter for encoding header values to base64
type encodeBase64 struct {
	typ        encodeBase64Type
	headerName string
	partIndex  *int // nil means encode entire value, otherwise encode the part at this index
}

// NewEncodeRequestHeaderBase64 returns a filter specification that encodes
// a request header value (or a specific space-delimited part of it) to base64.
// Instances expect one or two parameters:
// - First parameter: header name (required)
// - Second parameter: part index (optional, 0-based)
// If the second parameter is not provided, the entire header value is encoded.
// Name: "encodeBase64RequestHeader".
func NewEncodeRequestHeaderBase64() filters.Spec {
	return &encodeBase64{typ: encodeRequestHeaderBase64}
}

// NewEncodeResponseHeaderBase64 returns a filter specification that encodes
// a response header value (or a specific space-delimited part of it) to base64.
// Instances expect one or two parameters:
// - First parameter: header name (required)
// - Second parameter: part index (optional, 0-based)
// If the second parameter is not provided, the entire header value is encoded.
// Name: "encodeBase64ResponseHeader".
func NewEncodeResponseHeaderBase64() filters.Spec {
	return &encodeBase64{typ: encodeResponseHeaderBase64}
}

func (spec *encodeBase64) Name() string {
	switch spec.typ {
	case encodeRequestHeaderBase64:
		return filters.EncodeBase64RequestHeaderName
	case encodeResponseHeaderBase64:
		return filters.EncodeBase64ResponseHeaderName
	default:
		panic("invalid encodeBase64 type")
	}
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *encodeBase64) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	var partIndex *int
	if len(args) == 2 {
		switch v := args[1].(type) {
		case float64:
			idx := int(v)
			partIndex = &idx
		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &encodeBase64{
		typ:        spec.typ,
		headerName: headerName,
		partIndex:  partIndex,
	}, nil
}

// encodeBase64Value encodes a value (or a specific part of it) to base64.
// If partIndex is nil, it encodes the entire value.
// If partIndex is provided, it splits the value by whitespace and encodes the part at that index.
func encodeBase64Value(value string, partIndex *int) (string, error) {
	if partIndex == nil {
		// Encode the entire value
		encoded := base64.StdEncoding.EncodeToString([]byte(value))
		return encoded, nil
	}

	// Split by whitespace and encode the specified part
	parts := strings.Fields(value)
	if *partIndex < 0 || *partIndex >= len(parts) {
		return "", fmt.Errorf("part index %d out of range (header has %d parts)", *partIndex, len(parts))
	}

	// Encode the part at the given index
	parts[*partIndex] = base64.StdEncoding.EncodeToString([]byte(parts[*partIndex]))
	return strings.Join(parts, " "), nil
}

func (f *encodeBase64) Request(ctx filters.FilterContext) {
	if f.typ != encodeRequestHeaderBase64 {
		return
	}

	header := ctx.Request().Header
	headerValue := header.Get(f.headerName)
	if headerValue == "" {
		return
	}

	encoded, err := encodeBase64Value(headerValue, f.partIndex)
	if err != nil {
		ctx.Logger().Warnf("encodeBase64 filter error for header %s: %v", f.headerName, err)
		return
	}

	header.Set(f.headerName, encoded)
}

func (f *encodeBase64) Response(ctx filters.FilterContext) {
	if f.typ != encodeResponseHeaderBase64 {
		return
	}

	header := ctx.Response().Header
	headerValue := header.Get(f.headerName)
	if headerValue == "" {
		return
	}

	encoded, err := encodeBase64Value(headerValue, f.partIndex)
	if err != nil {
		ctx.Logger().Warnf("encodeBase64 filter error for header %s: %v", f.headerName, err)
		return
	}

	header.Set(f.headerName, encoded)
}
