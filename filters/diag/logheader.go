package diag

import (
	"bytes"
	"strings"

	"github.com/zalando/skipper/filters"
)

type logHeader struct {
	request  bool
	response bool
}

// NewLogHeader creates a filter specification for the 'logHeader()' filter.
func NewLogHeader() filters.Spec { return logHeader{} }

// Name returns the logHeader filter name.
func (logHeader) Name() string {
	return filters.LogHeaderName
}

func (logHeader) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		request  = false
		response = false
	)

	// default behavior
	if len(args) == 0 {
		request = true
	}

	for i := range args {
		opt, ok := args[i].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		switch strings.ToLower(opt) {
		case "response":
			response = true
		case "request":
			request = true
		}

	}

	return logHeader{
		request:  request,
		response: response,
	}, nil
}

func (lh logHeader) Response(ctx filters.FilterContext) {
	if !lh.response {
		return
	}

	req := ctx.Request()
	resp := ctx.Response()

	buf := bytes.NewBuffer(nil)
	buf.WriteString(req.Method)
	buf.WriteString(" ")
	buf.WriteString(req.URL.Path)
	buf.WriteString(" ")
	buf.WriteString(req.Proto)
	buf.WriteString("\r\n")
	buf.WriteString(resp.Status)
	buf.WriteString("\r\n")
	for k, v := range resp.Header {
		if strings.ToLower(k) == "authorization" {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString("TRUNCATED\r\n")
		} else {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString(strings.Join(v, " "))
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\r\n")

	ctx.Logger().Infof("Response for %s", buf.String())
}

func (lh logHeader) Request(ctx filters.FilterContext) {
	if !lh.request {
		return
	}

	req := ctx.Request()

	buf := bytes.NewBuffer(nil)
	buf.WriteString(req.Method)
	buf.WriteString(" ")
	buf.WriteString(req.URL.Path)
	buf.WriteString(" ")
	buf.WriteString(req.Proto)
	buf.WriteString("\r\nHost: ")
	buf.WriteString(req.Host)
	buf.WriteString("\r\n")
	for k, v := range req.Header {
		if strings.ToLower(k) == "authorization" {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString("TRUNCATED\r\n")
		} else {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString(strings.Join(v, " "))
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\r\n")

	ctx.Logger().Infof("%s", buf.String())
}
