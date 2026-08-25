// Package xforward provides filters to change XFF headers.
package xforward

import (
	"github.com/zalando/skipper/filters"
	snet "github.com/zalando/skipper/net"
)

const (
	// Deprecated, use filters.XforwardName instead
	Name = filters.XforwardName

	// Deprecated, use filters.XforwardFirstName instead
	NameFirst = filters.XforwardFirstName
)

type filter struct {
	headers *snet.ForwardedHeaders
}

// New creates a specification for the xforward filter
// that appends the remote IP of the incoming request to the
// X-Forwarded-For header, and sets the X-Forwarded-Host header
// to the value of the incoming request's Host header.
func New() filters.Spec {
	return filter{headers: &snet.ForwardedHeaders{For: true, Host: true}}
}

// NewFirst creates a specification for the xforwardFirst filter
// that prepends the remote IP of the incoming request to the
// X-Forwarded-For header, and sets the X-Forwarded-Host header
// to the value of the incoming request's Host header.
func NewFirst() filters.Spec {
	return filter{headers: &snet.ForwardedHeaders{PrependFor: true, Host: true}}
}

func (f filter) Name() string {
	if f.headers.PrependFor {
		return filters.XforwardFirstName
	}
	return filters.XforwardName
}

func (f filter) CreateFilter([]any) (filters.Filter, error) {
	return filter(f), nil
}

func (f filter) Request(ctx filters.FilterContext) {
	req := ctx.OriginalRequest()
	if req == nil {
		req = ctx.Request()
	}
	f.headers.Set(req)
}

func (filter) Response(filters.FilterContext) {}
