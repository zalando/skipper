package builtin

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/zalando/skipper/filters"
)

type inlineContentIfStatus struct {
	statusCode int
	text       string
	mime       string
}

// NewInlineContentIfStatus creates a filter spec for the inlineContent() filter.
//
//	r: * -> inlineContentIfStatus(401, "{\"foo\": 42}", "application/json") -> "https://www.example.org";
//
// It accepts three arguments: the statusCode code to match, the content and the optional content type.
// When the content type is not set, it tries to detect it using http.DetectContentType.
//
// The filter replaces the response coming from the backend or the following filters.
func NewInlineContentIfStatus() filters.Spec {
	return &inlineContentIfStatus{}
}

func (c *inlineContentIfStatus) Name() string { return filters.InlineContentIfStatusName }

func (c *inlineContentIfStatus) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var (
		f  inlineContentIfStatus
		ok bool
	)

	f.statusCode, ok = args[0].(int)
	if !ok {
		floatStatusCode, ok := args[0].(float64)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		f.statusCode = int(floatStatusCode)
	}

	if f.statusCode < 100 || f.statusCode >= 600 {
		return nil, filters.ErrInvalidFilterParameters
	}

	f.text, ok = args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(args) == 3 {
		f.mime, ok = args[2].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	} else {
		f.mime = http.DetectContentType([]byte(f.text))
	}

	return &f, nil
}

func (c *inlineContentIfStatus) Request(filters.FilterContext) {}

func (c *inlineContentIfStatus) Response(ctx filters.FilterContext) {
	if ctx.Response().StatusCode != c.statusCode {
		return
	}
	rsp := ctx.Response()

	err := rsp.Body.Close()
	if err != nil {
		ctx.Logger().Errorf("%v", err)
	}

	contentLength := len(c.text)
	rsp.ContentLength = int64(contentLength)
	rsp.Header.Set("Content-Type", c.mime)
	rsp.Header.Set("Content-Length", strconv.Itoa(contentLength))
	rsp.Body = io.NopCloser(bytes.NewBufferString(c.text))
}
