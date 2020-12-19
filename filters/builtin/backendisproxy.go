package builtin

import "github.com/zalando/skipper/filters"

const (
	BackendIsProxyName = "backendIsProxy"
)

type backendIsProxySpec struct{}

type backendIsProxyFilter struct{}

// NewBackendIsProxy returns a filter specification that is used to specify that the backend is also a proxy.
func NewBackendIsProxy() filters.Spec {
	return &backendIsProxySpec{}
}

func (s *backendIsProxySpec) Name() string {
	return BackendIsProxyName
}

func (s *backendIsProxySpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	return &backendIsProxyFilter{}, nil
}

func (f *backendIsProxyFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.BackendIsProxyKey] = struct{}{}
}

func (f *backendIsProxyFilter) Response(ctx filters.FilterContext) {
}
