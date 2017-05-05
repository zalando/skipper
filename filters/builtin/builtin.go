/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/filters/tee"
)

const (
	// Deprecated: use setRequestHeader or appendRequestHeader
	RequestHeaderName = "requestHeader"

	// Deprecated: use setRequestHeader or appendRequestHeader
	ResponseHeaderName = "responseHeader"

	// Deprecated: use redirectTo
	RedirectName = "redirect"

	SetRequestHeaderName     = "setRequestHeader"
	SetResponseHeaderName    = "setResponseHeader"
	AppendRequestHeaderName  = "appendRequestHeader"
	AppendResponseHeaderName = "appendResponseHeader"
	DropRequestHeaderName    = "dropRequestHeader"
	DropResponseHeaderName   = "dropResponseHeader"

	HealthCheckName  = "healthcheck"
	ModPathName      = "modPath"
	SetPathName      = "setPath"
	RedirectToName   = "redirectTo"
	StaticName       = "static"
	StripQueryName   = "stripQuery"
	PreserveHostName = "preserveHost"
	StatusName       = "status"
	CompressName     = "compress"
	SetQueryName     = "setQuery"
	DropQueryName    = "dropQuery"
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
		NewDropRequestHeader(),
		NewResponseHeader(),
		NewSetResponseHeader(),
		NewAppendResponseHeader(),
		NewDropResponseHeader(),
		NewModPath(),
		NewSetPath(),
		NewDropQuery(),
		NewSetQuery(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirect(),
		NewRedirectTo(),
		NewStripQuery(),
		flowid.New(),
		PreserveHost(),
		NewStatus(),
		NewCompress(),
		diag.NewRandom(),
		diag.NewLatency(),
		diag.NewBandwidth(),
		diag.NewChunks(),
		diag.NewBackendLatency(),
		diag.NewBackendBandwidth(),
		diag.NewBackendChunks(),
		tee.NewTee(),
		tee.NewTeeDeprecated(),
		tee.NewTeeNoFollow(),
		auth.NewBasicAuth(),
	} {
		r.Register(s)
	}

	return r
}
