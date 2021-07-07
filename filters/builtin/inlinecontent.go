package builtin

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

type inlineContent struct {
	template *eskip.Template
	mime     string
}

// Creates a filter spec for the inlineContent() filter.
//
// Usage of the filter:
//
//     * -> status(420) -> inlineContent("Enhance Your Calm") -> <shunt>
//
// Or:
//
//     * -> inlineContent("{\"foo\": 42}", "application/json") -> <shunt>
//
// It accepts two arguments: the content template (see eskip.Template.ApplyContext) and the optional content type.
// When the content type is not set, it tries to detect it using
// http.DetectContentType.
//
// The filter shunts the request with status code 200.
//
func NewInlineContent() filters.Spec {
	return &inlineContent{}
}

func (c *inlineContent) Name() string { return InlineContentName }

func (c *inlineContent) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var f inlineContent

	text, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	f.template = eskip.NewTemplate(text)

	if len(args) == 2 {
		f.mime, ok = args[1].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	} else {
		f.mime = http.DetectContentType([]byte(text))
	}

	return &f, nil
}

func (c *inlineContent) Request(ctx filters.FilterContext) {
	text, _ := c.template.ApplyContext(ctx)
	ctx.Serve(&http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{c.mime},
			"Content-Length": []string{strconv.Itoa(len(text))},
		},
		Body: ioutil.NopCloser(bytes.NewBufferString(text)),
	})
}

func (c *inlineContent) Response(filters.FilterContext) {}
