/*
Package sed provides stream editor filters for request and response payload.

The filter sed() expects a regexp pattern and a replacement string as arguments. During the streaming of the
response body, every occurence of the pattern will be replaced with the replacement string. The editing doesn't
happen right when the filter is executed, only later when the streaming normally happens, after all response
filters were called. The sed() filter accepts an optional third argument, the max editor buffer size in bytes.
This argument limits how much data can be buffered at a given time by the editor. The default value is 2MiB. See
more details below.

The filter uses the go regular expression implementation: https://github.com/google/re2/wiki/Syntax

Example:

	* -> sed("foo", "bar") -> "https://www.example.org"

The filter sedDelim() is like sed(), but it expects an additional argument, before the optional max buffer size
argument, that is used to delimit chunks to be processed at once. The pattern replacement is executed only
within the boundaries of the chunks defined by the delimiter, and matches across the chunk boundaries are not
considered.

Example:

	* -> sedDelim("foo", "bar", "\n") -> "https://www.example.org"

The filter sedRequest() is like sed(), but for the request content.

Example:

	* -> sedRequest("foo", "bar") -> "https://www.example.org"

The filter sedRequestDelim() is like sedDelim(), but for the request content.

Example:

	* -> sedRequestDelim("foo", "bar", "\n") -> "https://www.example.org"

Memory handling and limitations

In order to avoid unbound buffering of unprocessed data, the sed* filters need to apply some limitations. Some
patterns, e.g. `.*` would allow to match the complete payload, and it could result in trying to buffer it all
and potentially causing running out of available memory. Similarly, in case of certain expressions, when they
don't match, it's impossible to tell if they would match without reading more data from the source, and so would
potentially need to buffer the entire payload.

To prevent too high memory usage, the max buffer size is limited in case of each variant of the filter, by
default to 2MiB, which is the same limit as the one we apply when reading the request headers by default. When
the limit is reached, and the buffered content matches the pattern, then it is processed by replacing it, when
it doesn't match the pattern, then it is forwarded unchanged. This way, e.g. `sed(".*", "")` can be used safely
to consume and discard the payload.

As a result of this, with large payloads, it is possible that the resulting content will be different than if we
had run the replacement on the entire content at once. If we have enough preliminary knowledge about the
payload, then it may be better to use the delimited variant of the filters, e.g. for line based editing.
*/
package sed

import (
	"regexp"
	"strconv"

	"github.com/zalando/skipper/filters"
)

const (
	Name               = "sed"
	NameDelimit        = "sedDelim"
	NameRequest        = "sedRequest"
	NameRequestDelimit = "sedRequestDelim"
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
	typ             typ
	pattern         *regexp.Regexp
	replacement     []byte
	delimiter       []byte
	maxEditorBuffer int
}

func ofType(t typ) spec {
	return spec{typ: t}
}

// New creates a filter specficiation for the sed() filter.
func New() filters.Spec {
	return ofType(simple)
}

// NewDelimited creates a filter specficiation for the sedDelim() filter.
func NewDelimited() filters.Spec {
	return ofType(delimited)
}

// NewRequest creates a filter specficiation for the sedRequest() filter.
func NewRequest() filters.Spec {
	return ofType(simpleRequest)
}

// NewDelimitedRequest creates a filter specficiation for the sedRequestDelim() filter.
func NewDelimitedRequest() filters.Spec {
	return ofType(delimitedRequest)
}

func (s spec) Name() string {
	switch s.typ {
	case delimited:
		return NameDelimit
	case simpleRequest:
		return NameRequest
	case delimitedRequest:
		return NameRequestDelimit
	default:
		return Name
	}
}

func unescape(s string) string {
	return s
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

	f := filter{
		typ:         s.typ,
		pattern:     patternRx,
		replacement: []byte(replacement),
	}

	var delimiterString, maxBufString string
	switch s.typ {
	case delimited, delimitedRequest:
		if len(args) < 3 || len(args) > 4 {
			return nil, filters.ErrInvalidFilterParameters
		}

		if delimiterString, ok = args[2].(string); !ok {
			return nil, filters.ErrInvalidFilterParameters
		}

		if len(args) == 4 {
			if maxBufString, ok = args[3].(string); !ok {
				return nil, filters.ErrInvalidFilterParameters
			}
		}

		// Temporary solution, see eskip tokenizer bug: ...
		delimiterString = unescape(delimiterString)
		f.delimiter = []byte(delimiterString)
	default:
		if len(args) > 3 {
			return nil, filters.ErrInvalidFilterParameters
		}

		if len(args) == 3 {
			if maxBufString, ok = args[2].(string); !ok {
				return nil, filters.ErrInvalidFilterParameters
			}
		}
	}

	if maxBufString != "" {
		if f.maxEditorBuffer, err = strconv.Atoi(maxBufString); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (f filter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case simple, delimited:
		return
	}

	req := ctx.Request()
	req.Header.Del("Content-Length")
	req.ContentLength = -1
	req.Body = newEditor(req.Body, f.pattern, f.replacement, f.delimiter, f.maxEditorBuffer)
}

func (f filter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case simpleRequest, delimitedRequest:
		return
	}

	rsp := ctx.Response()
	rsp.Header.Del("Content-Length")
	rsp.ContentLength = -1
	rsp.Body = newEditor(rsp.Body, f.pattern, f.replacement, f.delimiter, f.maxEditorBuffer)
}
