package tls

import (
	"fmt"

	"github.com/zalando/skipper/filters"
)

type proxySSLVerifyOffSpec struct{}

type proxySSLVerifyOffFilter struct{}

// NewProxySSLVerifyOff creates a filter that disables TLS certificate verification
// for the backend on a per-route basis. Requires AllowInsecureBackend to be enabled
// globally in the proxy configuration; otherwise this filter has no effect.
func NewProxySSLVerifyOff() filters.Spec {
	return &proxySSLVerifyOffSpec{}
}

func (s *proxySSLVerifyOffSpec) Name() string {
	return filters.ProxySSLVerifyOffName
}

func (s *proxySSLVerifyOffSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%s: no arguments expected, got %d", filters.ProxySSLVerifyOffName, len(args))
	}
	return &proxySSLVerifyOffFilter{}, nil
}

func (f *proxySSLVerifyOffFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.BackendSkipTLSVerify] = true
}

func (f *proxySSLVerifyOffFilter) Response(filters.FilterContext) {}
