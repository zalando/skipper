package diag

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/zalando/skipper/filters"
)

type logBody struct {
	request  bool
	response bool
}

type xmlNode struct {
	Attr     []xml.Attr
	XMLName  xml.Name
	Children []xmlNode `xml:",any"`
	Text     string    `xml:",chardata"`
}

type LogMessage struct {
	Method      string
	Path        string
	Proto       string
	Status      string
	Body        string
	ContentType string
}

var (
	textPredicate = func(contentType string) bool {
		return strings.HasPrefix(contentType, "text/")
	}
	jsonPredicate = func(contentType string) bool {
		return strings.HasPrefix(contentType, "application/json") || strings.HasPrefix(contentType, "application/ld+json")
	}
	xmlPredicate = func(contentType string) bool {
		return strings.HasPrefix(contentType, "application/xml")
	}
)

// NewLogBody creates a filter specification for the 'logBody()' filter.
func NewLogBody() filters.Spec { return logBody{} }

// Name returns the logBody filter name.
func (logBody) Name() string {
	return filters.LogBodyName
}

func (logBody) CreateFilter(args []interface{}) (filters.Filter, error) {
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

	return logBody{
		request:  request,
		response: response,
	}, nil
}

func (lh logBody) Response(ctx filters.FilterContext) {
	if !lh.response {
		return
	}

	defer ctx.Response().Body.Close()

	req := ctx.Request()
	resp := ctx.Response()
	contentType := resp.Header.Get("Content-Type")

	body, err := io.ReadAll(ctx.Response().Body)
	if err != nil {
		return
	}
	m := LogMessage{
		Method:      req.Method,
		Path:        req.URL.Path,
		Proto:       req.Proto,
		Status:      resp.Status,
		Body:        string(body),
		ContentType: contentType,
	}

	buf := write(m)

	ctx.Logger().Infof("Response for %s", buf.String())
}

func (lh logBody) Request(ctx filters.FilterContext) {
	defer ctx.Request().Body.Close()

	req := ctx.Request()
	resp := ctx.Response()
	contentType := req.Header.Get("Content-Type")
	fmt.Printf("contentType: %s\n", contentType)

	body, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		return
	}
	m := LogMessage{
		Method:      req.Method,
		Path:        req.URL.Path,
		Proto:       req.Proto,
		Status:      resp.Status,
		Body:        string(body),
		ContentType: contentType,
	}
	buf := write(m)

	ctx.Logger().Infof("Request for %s", buf.String())
}

func write(message LogMessage) bytes.Buffer {
	var buf bytes.Buffer
	buf.WriteString(message.Method)
	buf.WriteString(" ")
	buf.WriteString(message.Path)
	buf.WriteString(" ")
	buf.WriteString(message.Proto)
	buf.WriteString("\r\n")
	buf.WriteString(message.Status)
	buf.WriteString("\r\n")
	if message.ContentType == "" {
		buf.WriteString("error: unrecognized content type. Body can't be logged.\n")
	} else if textPredicate(message.ContentType) {
		buf.WriteString(string(message.Body))
	} else if jsonPredicate(message.ContentType) {
		prettyJSON, err := json.MarshalIndent(message.Body, "", "  ")
		if err != nil {
			buf.WriteString("error: invalid json. Body can't be logged.\n")
			return buf
		}
		buf.WriteString(string(prettyJSON))
	} else if xmlPredicate(message.ContentType) {
		var node xmlNode
		err := xml.Unmarshal([]byte(message.Body), &node)
		if err != nil {
			buf.WriteString("error: invalid xml. Body can't be logged.\n")
			return buf
		}
		prettyXML, err := xml.MarshalIndent(node, "", " \t")
		if err != nil {
			buf.WriteString("error: invalid xml. Body can't be logged.\n")
			return buf
		}
		buf.WriteString(string(prettyXML))

	} else {
		buf.WriteString("error: content type doesn't support text representation. Body can't be logged.\n")
	}
	return buf
}
