/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/circuit"
	"github.com/zalando/skipper/filters/cookie"
	"github.com/zalando/skipper/filters/cors"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/flowid"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/filters/ratelimit"
	"github.com/zalando/skipper/filters/rfc"
	"github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/filters/xforward"
	"github.com/zalando/skipper/script"
)

const (
	SetRequestHeaderName            = "setRequestHeader"
	SetResponseHeaderName           = "setResponseHeader"
	AppendRequestHeaderName         = "appendRequestHeader"
	AppendResponseHeaderName        = "appendResponseHeader"
	DropRequestHeaderName           = "dropRequestHeader"
	DropResponseHeaderName          = "dropResponseHeader"
	SetContextRequestHeaderName     = "setContextRequestHeader"
	AppendContextRequestHeaderName  = "appendContextRequestHeader"
	SetContextResponseHeaderName    = "setContextResponseHeader"
	AppendContextResponseHeaderName = "appendContextResponseHeader"

	SetDynamicBackendHostFromHeader   = "setDynamicBackendHostFromHeader"
	SetDynamicBackendSchemeFromHeader = "setDynamicBackendSchemeFromHeader"
	SetDynamicBackendUrlFromHeader    = "setDynamicBackendUrlFromHeader"
	SetDynamicBackendHost             = "setDynamicBackendHost"
	SetDynamicBackendScheme           = "setDynamicBackendScheme"
	SetDynamicBackendUrl              = "setDynamicBackendUrl"

	HealthCheckName           = "healthcheck"
	ModPathName               = "modPath"
	SetPathName               = "setPath"
	ModRequestHeaderName      = "modRequestHeader"
	RedirectToName            = "redirectTo"
	RedirectToLowerName       = "redirectToLower"
	StaticName                = "static"
	StripQueryName            = "stripQuery"
	PreserveHostName          = "preserveHost"
	SetFastCgiFilenameName    = "setFastCgiFilename"
	StatusName                = "status"
	CompressName              = "compress"
	SetQueryName              = "setQuery"
	DropQueryName             = "dropQuery"
	InlineContentName         = "inlineContent"
	InlineContentIfStatusName = "inlineContentIfStatus"
	HeaderToQueryName         = "headerToQuery"
	QueryToHeaderName         = "queryToHeader"
)

// Returns a Registry object initialized with the default set of filter
// specifications found in the filters package. (including the builtin
// and the flowid subdirectories.)
func MakeRegistry() filters.Registry {
	r := make(filters.Registry)
	for _, s := range []filters.Spec{
		NewBackendIsProxy(),
		NewSetRequestHeader(),
		NewAppendRequestHeader(),
		NewDropRequestHeader(),
		NewSetResponseHeader(),
		NewAppendResponseHeader(),
		NewDropResponseHeader(),
		NewSetContextRequestHeader(),
		NewAppendContextRequestHeader(),
		NewSetContextResponseHeader(),
		NewAppendContextResponseHeader(),
		NewModPath(),
		NewSetPath(),
		NewModRequestHeader(),
		NewDropQuery(),
		NewSetQuery(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirectTo(),
		NewRedirectLower(),
		NewStripQuery(),
		NewInlineContent(),
		NewInlineContentIfStatus(),
		flowid.New(),
		xforward.New(),
		xforward.NewFirst(),
		PreserveHost(),
		NewSetFastCgiFilename(),
		NewStatus(),
		NewCompress(),
		NewDecompress(),
		NewCopyRequestHeader(),
		NewCopyResponseHeader(),
		NewHeaderToQuery(),
		NewQueryToHeader(),
		NewSetDynamicBackendHostFromHeader(),
		NewSetDynamicBackendSchemeFromHeader(),
		NewSetDynamicBackendUrlFromHeader(),
		NewSetDynamicBackendHost(),
		NewSetDynamicBackendScheme(),
		NewSetDynamicBackendUrl(),
		NewOriginMarkerSpec(),
		diag.NewRandom(),
		diag.NewLatency(),
		diag.NewBandwidth(),
		diag.NewChunks(),
		diag.NewBackendLatency(),
		diag.NewBackendBandwidth(),
		diag.NewBackendChunks(),
		diag.NewAbsorb(),
		diag.NewLogHeader(),
		tee.NewTee(),
		tee.NewTeeNoFollow(),
		sed.New(),
		sed.NewDelimited(),
		sed.NewRequest(),
		sed.NewDelimitedRequest(),
		auth.NewBasicAuth(),
		cookie.NewRequestCookie(),
		cookie.NewResponseCookie(),
		cookie.NewJSCookie(),
		circuit.NewConsecutiveBreaker(),
		circuit.NewRateBreaker(),
		circuit.NewDisableBreaker(),
		ratelimit.NewClientRatelimit(),
		ratelimit.NewRatelimit(),
		ratelimit.NewClusterRateLimit(),
		ratelimit.NewClusterClientRateLimit(),
		ratelimit.NewDisableRatelimit(),
		script.NewLuaScript(),
		cors.NewOrigin(),
		logfilter.NewUnverifiedAuditLog(),
		tracing.NewSpanName(),
		tracing.NewBaggageToTagFilter(),
		tracing.NewTag(),
		accesslog.NewDisableAccessLog(),
		accesslog.NewEnableAccessLog(),
		auth.NewForwardToken(),
		scheduler.NewLIFO(),
		scheduler.NewLIFOGroup(),
		rfc.NewPath(),
	} {
		r.Register(s)
	}

	return r
}
