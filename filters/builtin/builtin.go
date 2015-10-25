/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
)

const (
	RequestHeaderName  = "requestHeader"
	ResponseHeaderName = "responseHeader"
	HealthCheckName    = "healthcheck"
	ModPathName        = "modPath"
	RedirectName       = "redirect"
	StaticName         = "static"
	StripQueryName     = "stripQuery"
)

// Returns a Registry object initialized with the default set of filter
// specifications found in the filters package. (including the builtin
// and the flowid subdirectories.)
func MakeRegistry() filters.Registry {
	r := make(filters.Registry)
	for _, s := range []filters.Spec{
		NewRequestHeader(),
		NewResponseHeader(),
		NewModPath(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirect(),
		NewStripQuery(),
		flowid.New(),
	} {
		r.Register(s)
	}

	return r
}
