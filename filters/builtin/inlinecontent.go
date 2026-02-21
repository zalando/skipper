package builtin

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/zalando/skipper/filters"
)

type inlineContent struct {
	text string
	mime string
}

// NewInlineContent creates a filter spec for the inlineContent() filter.
//
// Usage of the filter:
//
//	r: * -> status(420) -> inlineContent("Enhance Your Calm") -> <shunt>;
//
// Or:
//
//	r: * -> inlineContent("{\"foo\": 42}", "application/json") -> <shunt>;
//
// It accepts two arguments: the content and the optional content type.
// When the content type is not set, it tries to detect it using
// http.DetectContentType.
//
// The filter shunts the request with status code 200.
func NewInlineContent() filters.Spec {
	return &inlineContent{}
}

func (c *inlineContent) Name() string { return filters.InlineContentName }

func stringArg(a any) (s string, err error) {
	var ok bool
	s, ok = a.(string)
	if !ok {
		err = filters.ErrInvalidFilterParameters
	}

	return
}

func (c *inlineContent) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var (
		f   inlineContent
		err error
	)

	f.text, err = stringArg(args[0])
	if err != nil {
		return nil, err
	}

	if len(args) == 2 {
		f.mime, err = stringArg(args[1])
		if err != nil {
			return nil, err
		}
	} else {
		f.mime = http.DetectContentType([]byte(f.text))
	}

	return &f, nil
}

func (c *inlineContent) Request(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{c.mime},
			"Content-Length": []string{strconv.Itoa(len(c.text))},
		},
		Body: io.NopCloser(bytes.NewBufferString(c.text)),
	})
}

func (c *inlineContent) Response(filters.FilterContext) {}
