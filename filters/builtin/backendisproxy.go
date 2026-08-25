package builtin

import "github.com/zalando/skipper/filters"

type backendIsProxySpec struct{}

type backendIsProxyFilter struct{}

// NewBackendIsProxy returns a filter specification that is used to specify that the backend is also a proxy.
func NewBackendIsProxy() filters.Spec {
	return &backendIsProxySpec{}
}

func (s *backendIsProxySpec) Name() string {
	return filters.BackendIsProxyName
}

func (s *backendIsProxySpec) CreateFilter(args []any) (filters.Filter, error) {
	return &backendIsProxyFilter{}, nil
}

func (f *backendIsProxyFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.BackendIsProxyKey] = struct{}{}
}

func (f *backendIsProxyFilter) Response(ctx filters.FilterContext) {
}
