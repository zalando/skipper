package rfc

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/rfc"
)

const (
	Name     = "rfcPath"
	NameHost = "rfcHost"
)

type path struct{}

// NewPath creates a filter specification for the rfcPath() filter, that
// reencodes the reserved characters in the request path, if it detects
// that they are encoded in the raw path.
//
// See also the PatchPath documentation in the rfc package.
//
func NewPath() filters.Spec { return path{} }

func (p path) Name() string                                       { return Name }
func (p path) CreateFilter([]interface{}) (filters.Filter, error) { return path{}, nil }
func (p path) Response(filters.FilterContext)                     {}

func (p path) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	req.URL.Path = rfc.PatchPath(req.URL.Path, req.URL.RawPath)
}

type host struct{}

// NewHost creates a filter specification for the rfcHost() filter, that
// removes a trailing dot in the host header.
//
// See also the PatchHost documentation in the rfc package.
//
func NewHost() filters.Spec { return host{} }

func (host) Name() string                                       { return NameHost }
func (host) CreateFilter([]interface{}) (filters.Filter, error) { return host{}, nil }
func (host) Response(filters.FilterContext)                     {}

func (host) Request(ctx filters.FilterContext) {
	ctx.Request().Host = rfc.PatchHost(ctx.Request().Host)
	ctx.SetOutgoingHost(rfc.PatchHost(ctx.OutgoingHost()))
}
