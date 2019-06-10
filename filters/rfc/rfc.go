package rfc

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/rfc"
)

const Name = "rfcPath"

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
