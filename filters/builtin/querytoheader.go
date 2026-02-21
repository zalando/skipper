package builtin

import (
	"fmt"

	"github.com/zalando/skipper/filters"
)

type (
	queryToHeaderSpec struct {
	}

	queryToHeaderFilter struct {
		headerName     string
		queryParamName string
		formatString   string
	}
)

// NewQueryToHeader creates a filter which converts query params
// from the incoming Request to headers
//
//	queryToHeader("foo-query-param", "X-Foo-Header")
//
// The above filter will set the value of "X-Foo-Header" header to the
// value of "foo-query-param" query param , to the request and will
// not override the value if the header exists already
//
// The header value can be created by a formatstring with an optional third parameter
//
//	queryToHeader("foo-query-param", "X-Foo-Header", "prefix %s postfix")
//	queryToHeader("access_token", "Authorization", "Bearer %s")
func NewQueryToHeader() filters.Spec {
	return &queryToHeaderSpec{}
}

func (*queryToHeaderSpec) Name() string {
	return filters.QueryToHeaderName
}

// CreateFilter creates a `queryToHeader` filter instance with below signature
// s.CreateFilter("foo-query-param", "X-Foo-Header")
func (*queryToHeaderSpec) CreateFilter(args []any) (filters.Filter, error) {
	if l := len(args); l < 2 || l > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	q, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	h, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	formatString := "%s"
	if len(args) == 3 {
		formatString, ok = args[2].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return &queryToHeaderFilter{headerName: h, queryParamName: q, formatString: formatString}, nil
}

func (f *queryToHeaderFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	headerValue := req.Header.Get(f.headerName)
	if headerValue != "" {
		return
	}

	v := req.URL.Query().Get(f.queryParamName)
	if v == "" {
		return
	}

	req.Header.Set(f.headerName, fmt.Sprintf(f.formatString, v))
}

func (*queryToHeaderFilter) Response(ctx filters.FilterContext) {}
