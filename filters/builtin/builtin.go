/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/circuit"
	"github.com/zalando/skipper/filters/consistenthash"
	"github.com/zalando/skipper/filters/cookie"
	"github.com/zalando/skipper/filters/cors"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/filters/fadein"
	"github.com/zalando/skipper/filters/flowid"
	logfilter "github.com/zalando/skipper/filters/log"
	"github.com/zalando/skipper/filters/rfc"
	"github.com/zalando/skipper/filters/scheduler"
	"github.com/zalando/skipper/filters/sed"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/filters/tracing"
	"github.com/zalando/skipper/filters/xforward"
	"github.com/zalando/skipper/script"
)

const (
	// Deprecated: use setRequestHeader or appendRequestHeader
	RequestHeaderName = "requestHeader"

	// Deprecated: use setResponseHeader or appendResponseHeader
	ResponseHeaderName = "responseHeader"

	// Deprecated: use redirectTo
	RedirectName = "redirect"

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
	CopyRequestHeaderName           = "copyRequestHeader"
	CopyResponseHeaderName          = "copyResponseHeader"

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
	BackendTimeoutName        = "backendTimeout"
)

// Returns a Registry object initialized with the default set of filter
// specifications found in the filters package. (including the builtin
// and the flowid subdirectories.)
func MakeRegistry() filters.Registry {
	r := make(filters.Registry)
	for _, s := range []filters.Spec{
		NewBackendIsProxy(),
		NewRequestHeader(),
		NewSetRequestHeader(),
		NewAppendRequestHeader(),
		NewDropRequestHeader(),
		NewResponseHeader(),
		NewSetResponseHeader(),
		NewAppendResponseHeader(),
		NewDropResponseHeader(),
		NewSetContextRequestHeader(),
		NewAppendContextRequestHeader(),
		NewSetContextResponseHeader(),
		NewAppendContextResponseHeader(),
		NewCopyRequestHeader(),
		NewCopyResponseHeader(),
		NewCopyRequestHeaderDeprecated(),
		NewCopyResponseHeaderDeprecated(),
		NewModPath(),
		NewSetPath(),
		NewModRequestHeader(),
		NewDropQuery(),
		NewSetQuery(),
		NewHealthCheck(),
		NewStatic(),
		NewRedirect(),
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
		NewHeaderToQuery(),
		NewQueryToHeader(),
		NewBackendTimeout(),
		NewSetDynamicBackendHostFromHeader(),
		NewSetDynamicBackendSchemeFromHeader(),
		NewSetDynamicBackendUrlFromHeader(),
		NewSetDynamicBackendHost(),
		NewSetDynamicBackendScheme(),
		NewSetDynamicBackendUrl(),
		NewOriginMarkerSpec(),
		diag.NewRandom(),
		diag.NewRepeat(),
		diag.NewLatency(),
		diag.NewBandwidth(),
		diag.NewChunks(),
		diag.NewBackendLatency(),
		diag.NewBackendBandwidth(),
		diag.NewBackendChunks(),
		diag.NewAbsorb(),
		diag.NewAbsorbSilent(),
		diag.NewLogHeader(),
		tee.NewTee(),
		tee.NewTeeDeprecated(),
		tee.NewTeeNoFollow(),
		tee.NewTeeLoopback(),
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
		script.NewLuaScript(),
		cors.NewOrigin(),
		logfilter.NewUnverifiedAuditLog(),
		tracing.NewSpanName(),
		tracing.NewBaggageToTagFilter(),
		tracing.NewTag(),
		tracing.NewStateBagToTag(),
		accesslog.NewAccessLogDisabled(),
		accesslog.NewDisableAccessLog(),
		accesslog.NewEnableAccessLog(),
		auth.NewForwardToken(),
		scheduler.NewLIFO(),
		scheduler.NewLIFOGroup(),
		rfc.NewPath(),
		fadein.NewFadeIn(),
		fadein.NewEndpointCreated(),
		consistenthash.NewConsistentHashKey(),
	} {
		r.Register(s)
	}

	return r
}
