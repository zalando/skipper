package builtin

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/zalando/skipper/filters"
)

type decodeBase64Type int

const (
	decodeRequestHeaderBase64 decodeBase64Type = iota
	decodeResponseHeaderBase64
)

// decodeBase64 filter for decoding base64-encoded parts of headers
type decodeBase64 struct {
	typ        decodeBase64Type
	headerName string
	partIndex  *int // nil means decode entire value, otherwise decode the part at this index
}

// NewDecodeRequestHeaderBase64 returns a filter specification that decodes
// base64-encoded request header values or specific parts of them.
// Instances expect one or two parameters:
// - First parameter: header name (required)
// - Second parameter: part index (optional, 0-based)
// If the second parameter is not provided, the entire header value is decoded.
// Name: "decodeRequestHeaderBase64".
func NewDecodeRequestHeaderBase64() filters.Spec {
	return &decodeBase64{typ: decodeRequestHeaderBase64}
}

// NewDecodeResponseHeaderBase64 returns a filter specification that decodes
// base64-encoded response header values or specific parts of them.
// Instances expect one or two parameters:
// - First parameter: header name (required)
// - Second parameter: part index (optional, 0-based)
// If the second parameter is not provided, the entire header value is decoded.
// Name: "decodeResponseHeaderBase64".
func NewDecodeResponseHeaderBase64() filters.Spec {
	return &decodeBase64{typ: decodeResponseHeaderBase64}
}

func (spec *decodeBase64) Name() string {
	switch spec.typ {
	case decodeRequestHeaderBase64:
		return filters.DecodeBase64RequestHeaderName
	case decodeResponseHeaderBase64:
		return filters.DecodeBase64ResponseHeaderName
	default:
		panic("invalid decodeBase64 type")
	}
}

//lint:ignore ST1016 "spec" makes sense here and we reuse the type for the filter
func (spec *decodeBase64) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	var partIndex *int
	if len(args) == 2 {
		// Second parameter is optional and should be a number representing the part index
		switch v := args[1].(type) {
		case float64:
			idx := int(v)
			partIndex = &idx
		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &decodeBase64{
		typ:        spec.typ,
		headerName: headerName,
		partIndex:  partIndex,
	}, nil
}

// decodeBase64Value decodes a base64-encoded string or a specific part of it.
// If partIndex is nil, it decodes the entire value.
// If partIndex is provided, it splits the value by spaces and decodes the part at that index.
func decodeBase64Value(value string, partIndex *int) (string, error) {
	if partIndex == nil {
		// Decode the entire value
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64: %w", err)
		}
		return string(decoded), nil
	}

	// Split by spaces and decode the specified part
	parts := strings.Fields(value)
	if *partIndex < 0 || *partIndex >= len(parts) {
		return "", fmt.Errorf("part index %d out of range (header has %d parts)", *partIndex, len(parts))
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[*partIndex])
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 part at index %d: %w", *partIndex, err)
	}

	// Reconstruct the header with the decoded part
	parts[*partIndex] = string(decoded)
	return strings.Join(parts, " "), nil
}

func (f *decodeBase64) Request(ctx filters.FilterContext) {
	if f.typ != decodeRequestHeaderBase64 {
		return
	}

	header := ctx.Request().Header
	headerValue := header.Get(f.headerName)
	if headerValue == "" {
		return
	}

	decoded, err := decodeBase64Value(headerValue, f.partIndex)
	if err != nil {
		// Log error but continue - invalid base64 shouldn't break the request
		ctx.Logger().Warnf("decodeBase64 filter error for header %s: %v", f.headerName, err)
		return
	}

	header.Set(f.headerName, decoded)
}

func (f *decodeBase64) Response(ctx filters.FilterContext) {
	if f.typ != decodeResponseHeaderBase64 {
		return
	}

	header := ctx.Response().Header
	headerValue := header.Get(f.headerName)
	if headerValue == "" {
		return
	}

	decoded, err := decodeBase64Value(headerValue, f.partIndex)
	if err != nil {
		// Log error but continue - invalid base64 shouldn't break the response
		ctx.Logger().Warnf("decodeBase64 filter error for header %s: %v", f.headerName, err)
		return
	}

	header.Set(f.headerName, decoded)
}
