package sed

import (
	"regexp"

	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.SedName instead
	Name = filters.SedName
	// Deprecated, use filters.SedDelimName instead
	NameDelimit = filters.SedDelimName
	// Deprecated, use filters.SedRequestName instead
	NameRequest = filters.SedRequestName
	// Deprecated, use filters.SedRequestDelimName instead
	NameRequestDelimit = filters.SedRequestDelimName
)

type typ int

const (
	simple typ = iota
	delimited
	simpleRequest
	delimitedRequest
)

type spec struct {
	typ typ
}

type filter struct {
	typ               typ
	pattern           *regexp.Regexp
	replacement       []byte
	delimiter         []byte
	maxEditorBuffer   int
	maxBufferHandling maxBufferHandling
}

func ofType(t typ) spec {
	return spec{typ: t}
}

// New creates a filter specification for the sed() filter.
func New() filters.Spec {
	return ofType(simple)
}

// NewDelimited creates a filter specification for the sedDelim() filter.
func NewDelimited() filters.Spec {
	return ofType(delimited)
}

// NewRequest creates a filter specification for the sedRequest() filter.
func NewRequest() filters.Spec {
	return ofType(simpleRequest)
}

// NewDelimitedRequest creates a filter specification for the sedRequestDelim() filter.
func NewDelimitedRequest() filters.Spec {
	return ofType(delimitedRequest)
}

func (s spec) Name() string {
	switch s.typ {
	case delimited:
		return filters.SedDelimName
	case simpleRequest:
		return filters.SedRequestName
	case delimitedRequest:
		return filters.SedRequestDelimName
	default:
		return filters.SedName
	}
}

func parseMaxBufferHandling(h interface{}) (maxBufferHandling, error) {
	switch h {
	case "best-effort":
		return maxBufferBestEffort, nil
	case "abort":
		return maxBufferAbort, nil
	default:
		return 0, filters.ErrInvalidFilterParameters
	}
}

func (s spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	pattern, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	patternRx, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	replacement, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &filter{
		typ:               s.typ,
		pattern:           patternRx,
		replacement:       []byte(replacement),
		maxBufferHandling: maxBufferBestEffort,
	}

	var (
		delimiterString string
		maxBuf          interface{}
		maxBufHandling  interface{}
	)

	switch s.typ {
	case delimited, delimitedRequest:
		if len(args) < 3 || len(args) > 5 {
			return nil, filters.ErrInvalidFilterParameters
		}

		if delimiterString, ok = args[2].(string); !ok {
			return nil, filters.ErrInvalidFilterParameters
		}

		if len(args) >= 4 {
			maxBuf = args[3]
		}

		if len(args) == 5 {
			maxBufHandling = args[4]
		}

		f.delimiter = []byte(delimiterString)
	default:
		if len(args) > 4 {
			return nil, filters.ErrInvalidFilterParameters
		}

		if len(args) >= 3 {
			maxBuf = args[2]
		}

		if len(args) == 4 {
			maxBufHandling = args[3]
		}
	}

	if maxBuf != nil {
		switch v := maxBuf.(type) {
		case int:
			f.maxEditorBuffer = v
		case float64:
			f.maxEditorBuffer = int(v)
		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	if maxBufHandling != nil {
		mbh, err := parseMaxBufferHandling(maxBufHandling)
		if err != nil {
			return nil, err
		}

		f.maxBufferHandling = mbh
	}

	return *f, nil
}

func (f filter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case simple, delimited:
		return
	}

	req := ctx.Request()
	req.Header.Del("Content-Length")
	req.ContentLength = -1
	req.Body = newEditor(
		req.Body,
		f.pattern,
		f.replacement,
		f.delimiter,
		f.maxEditorBuffer,
		f.maxBufferHandling,
	)
}

func (f filter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case simpleRequest, delimitedRequest:
		return
	}

	rsp := ctx.Response()
	rsp.Header.Del("Content-Length")
	rsp.ContentLength = -1
	rsp.Body = newEditor(
		rsp.Body,
		f.pattern,
		f.replacement,
		f.delimiter,
		f.maxEditorBuffer,
		f.maxBufferHandling,
	)
}
