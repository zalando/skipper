/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
)

const (
	RequestHeaderName        = "requestHeader"
	ResponseHeaderName       = "responseHeader"
	SetRequestHeaderName     = "setRequestHeader"
	SetResponseHeaderName    = "setResponseHeader"
	AppendRequestHeaderName  = "appendRequestHeader"
	AppendResponseHeaderName = "appendResponseHeader"
	HealthCheckName          = "healthcheck"
	ModPathName              = "modPath"
	RedirectName             = "redirect"
	RedirectToName           = "redirectTo"
	StaticName               = "static"
	StripQueryName           = "stripQuery"
	PreserveHostName         = "preserveHost"
)

// Returns a Registry object initialized with the default set of filter
// specifications found in the filters package. (including the builtin
// and the flowid subdirectories.)
func MakeRegistry() filters.Registry {
	r := make(filters.Registry)
	for _, s := range []filters.Spec{
		NewRequestHeader(),
		NewSetRequestHeader(),
		NewAppendRequestHeader(),
		NewResponseHeader(),
		NewSetResponseHeader(),
		NewAppendResponseHeader(),
		NewModPath(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirect(),
		NewRedirectTo(),
		NewStripQuery(),
		flowid.New(),
		PreserveHost(),
	} {
		r.Register(s)
	}

	return r
}
