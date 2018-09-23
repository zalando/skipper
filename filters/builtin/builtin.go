/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/circuit"
	"github.com/zalando/skipper/filters/cookie"
	"github.com/zalando/skipper/filters/cors"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/flowid"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/filters/ratelimit"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/script"
)

const (
	// Deprecated: use setRequestHeader or appendRequestHeader
	RequestHeaderName = "requestHeader"

	// Deprecated: use setResponseHeader or appendResponseHeader
	ResponseHeaderName = "responseHeader"

	// Deprecated: use redirectTo
	RedirectName = "redirect"

	SetRequestHeaderName     = "setRequestHeader"
	SetResponseHeaderName    = "setResponseHeader"
	AppendRequestHeaderName  = "appendRequestHeader"
	AppendResponseHeaderName = "appendResponseHeader"
	DropRequestHeaderName    = "dropRequestHeader"
	DropResponseHeaderName   = "dropResponseHeader"

	HealthCheckName     = "healthcheck"
	ModPathName         = "modPath"
	SetPathName         = "setPath"
	RedirectToName      = "redirectTo"
	RedirectToLowerName = "redirectToLower"
	StaticName          = "static"
	StripQueryName      = "stripQuery"
	PreserveHostName    = "preserveHost"
	StatusName          = "status"
	CompressName        = "compress"
	SetQueryName        = "setQuery"
	DropQueryName       = "dropQuery"
	InlineContentName   = "inlineContent"
	HeaderToQueryName   = "headerToQuery"
	QueryToHeaderName   = "queryToHeader"
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
		NewRedirectLower(),
		NewStripQuery(),
		NewInlineContent(),
		flowid.New(),
		PreserveHost(),
		NewStatus(),
		NewCompress(),
		NewCopyRequestHeader(),
		NewCopyResponseHeader(),
		NewHeaderToQuery(),
		NewQueryToHeader(),
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
		cookie.NewRequestCookie(),
		cookie.NewResponseCookie(),
		cookie.NewJSCookie(),
		circuit.NewConsecutiveBreaker(),
		circuit.NewRateBreaker(),
		circuit.NewDisableBreaker(),
		ratelimit.NewLocalRatelimit(),
		ratelimit.NewRatelimit(),
		ratelimit.NewDisableRatelimit(),
		loadbalancer.NewDecide(),
		script.NewLuaScript(),
		cors.NewOrigin(),
		logfilter.NewUnverifiedAuditLog(),
		tracing.NewSpanName(),
		auth.NewForwardToken(),
	} {
		r.Register(s)
	}

	return r
}
