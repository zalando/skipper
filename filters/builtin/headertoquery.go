package builtin

import (
	"github.com/zalando/skipper/filters"
)

type (
	headerToQuerySpec struct {
	}

	headerToQueryFilter struct {
		headerName     string
		queryParamName string
	}
)

// NewHeaderToQuery creates a filter which converts the headers
// from the incoming Request to query params
//
//	headerToQuery("X-Foo-Header", "foo-query-param")
//
// The above filter will set the "foo-query-param" query param
// to the value of "X-Foo-Header" header, to the request
// and will override the value if the queryparam exists already
func NewHeaderToQuery() filters.Spec {
	return &headerToQuerySpec{}
}

func (*headerToQuerySpec) Name() string {
	return filters.HeaderToQueryName
}

// CreateFilter creates a `headerToQuery` filter instance with below signature
// s.CreateFilter("X-Foo-Header", "foo-query-param")
func (*headerToQuerySpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	h, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	q, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &headerToQueryFilter{h, q}, nil
}

func (f *headerToQueryFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	params := req.URL.Query()

	headerValue := req.Header.Get(f.headerName)
	params.Set(f.queryParamName, headerValue)

	req.URL.RawQuery = params.Encode()
}

func (*headerToQueryFilter) Response(ctx filters.FilterContext) {}
