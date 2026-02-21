package builtin

import "github.com/zalando/skipper/filters"

type loopbackIfStatusSpec struct {
}
type loopbackIfStatusFilter struct {
	statusCode int
	path       string
}

// NewLoopbackIfStatus Creates a filter spec for the loopbackIfStatus() filter.
//
//	r: * -> loopbackIfStatus(401, "/new-path") -> "https://www.example.org";
//
// It accepts two arguments: the statusCode code to and the path to change the request to.
//
// The filter replaces the response coming from the backend or the following filters.
func NewLoopbackIfStatus() filters.Spec {
	return &loopbackIfStatusSpec{}
}

func (s *loopbackIfStatusSpec) Name() string {
	return filters.LoopbackIfStatus
}

func (s *loopbackIfStatusSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var (
		f  loopbackIfStatusFilter
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

	f.path, ok = args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &f, nil
}

func (f *loopbackIfStatusFilter) Request(ctx filters.FilterContext) {

}

func (f *loopbackIfStatusFilter) Response(ctx filters.FilterContext) {
	if ctx.Response().StatusCode == f.statusCode {
		ctx.Request().URL.Path = f.path
		ctx.LoopbackWithResponse()
	}
}
