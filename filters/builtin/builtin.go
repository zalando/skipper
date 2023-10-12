/*
Package builtin provides a small, generic set of filters.
*/
package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/accesslog"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/awssigner/awssigv4"
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
	"github.com/zalando/skipper/filters/tls"
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

	// Deprecated, use filters.SetRequestHeaderName instead
	SetRequestHeaderName = filters.SetRequestHeaderName
	// Deprecated, use filters.SetResponseHeaderName instead
	SetResponseHeaderName = filters.SetResponseHeaderName
	// Deprecated, use filters.AppendRequestHeaderName instead
	AppendRequestHeaderName = filters.AppendRequestHeaderName
	// Deprecated, use filters.AppendResponseHeaderName instead
	AppendResponseHeaderName = filters.AppendResponseHeaderName
	// Deprecated, use filters.DropRequestHeaderName instead
	DropRequestHeaderName = filters.DropRequestHeaderName
	// Deprecated, use filters.DropResponseHeaderName instead
	DropResponseHeaderName = filters.DropResponseHeaderName
	// Deprecated, use filters.SetContextRequestHeaderName instead
	SetContextRequestHeaderName = filters.SetContextRequestHeaderName
	// Deprecated, use filters.AppendContextRequestHeaderName instead
	AppendContextRequestHeaderName = filters.AppendContextRequestHeaderName
	// Deprecated, use filters.SetContextResponseHeaderName instead
	SetContextResponseHeaderName = filters.SetContextResponseHeaderName
	// Deprecated, use filters.AppendContextResponseHeaderName instead
	AppendContextResponseHeaderName = filters.AppendContextResponseHeaderName
	// Deprecated, use filters.CopyRequestHeaderName instead
	CopyRequestHeaderName = filters.CopyRequestHeaderName
	// Deprecated, use filters.CopyResponseHeaderName instead
	CopyResponseHeaderName = filters.CopyResponseHeaderName

	// Deprecated, use filters.SetDynamicBackendHostFromHeader instead
	SetDynamicBackendHostFromHeader = filters.SetDynamicBackendHostFromHeader
	// Deprecated, use filters.SetDynamicBackendSchemeFromHeader instead
	SetDynamicBackendSchemeFromHeader = filters.SetDynamicBackendSchemeFromHeader
	// Deprecated, use filters.SetDynamicBackendUrlFromHeader instead
	SetDynamicBackendUrlFromHeader = filters.SetDynamicBackendUrlFromHeader
	// Deprecated, use filters.SetDynamicBackendHost instead
	SetDynamicBackendHost = filters.SetDynamicBackendHost
	// Deprecated, use filters.SetDynamicBackendScheme instead
	SetDynamicBackendScheme = filters.SetDynamicBackendScheme
	// Deprecated, use filters.SetDynamicBackendUrl instead
	SetDynamicBackendUrl = filters.SetDynamicBackendUrl

	// Deprecated, use filters.HealthCheckName instead
	HealthCheckName = filters.HealthCheckName
	// Deprecated, use filters.ModPathName instead
	ModPathName = filters.ModPathName
	// Deprecated, use filters.SetPathName instead
	SetPathName = filters.SetPathName
	// Deprecated, use filters.ModRequestHeaderName instead
	ModRequestHeaderName = filters.ModRequestHeaderName
	// Deprecated, use filters.RedirectToName instead
	RedirectToName = filters.RedirectToName
	// Deprecated, use filters.RedirectToLowerName instead
	RedirectToLowerName = filters.RedirectToLowerName
	// Deprecated, use filters.StaticName instead
	StaticName = filters.StaticName
	// Deprecated, use filters.StripQueryName instead
	StripQueryName = filters.StripQueryName
	// Deprecated, use filters.PreserveHostName instead
	PreserveHostName = filters.PreserveHostName
	// Deprecated, use filters.SetFastCgiFilenameName instead
	SetFastCgiFilenameName = filters.SetFastCgiFilenameName
	// Deprecated, use filters.StatusName instead
	StatusName = filters.StatusName
	// Deprecated, use filters.CompressName instead
	CompressName = filters.CompressName
	// Deprecated, use filters.SetQueryName instead
	SetQueryName = filters.SetQueryName
	// Deprecated, use filters.DropQueryName instead
	DropQueryName = filters.DropQueryName
	// Deprecated, use filters.InlineContentName instead
	InlineContentName = filters.InlineContentName
	// Deprecated, use filters.InlineContentIfStatusName instead
	InlineContentIfStatusName = filters.InlineContentIfStatusName
	// Deprecated, use filters.HeaderToQueryName instead
	HeaderToQueryName = filters.HeaderToQueryName
	// Deprecated, use filters.QueryToHeaderName instead
	QueryToHeaderName = filters.QueryToHeaderName
	// Deprecated, use filters.BackendTimeoutName instead
	BackendTimeoutName = filters.BackendTimeoutName
)

func Filters() []filters.Spec {
	return []filters.Spec{
		NewBackendIsProxy(),
		NewComment(),
		annotate.New(),
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
		NewModResponseHeader(),
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
		NewLoopbackIfStatus(),
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
		NewReadTimeout(),
		NewWriteTimeout(),
		NewSetDynamicBackendHostFromHeader(),
		NewSetDynamicBackendSchemeFromHeader(),
		NewSetDynamicBackendUrlFromHeader(),
		NewSetDynamicBackendHost(),
		NewSetDynamicBackendScheme(),
		NewSetDynamicBackendUrl(),
		NewOriginMarkerSpec(),
		awssigv4.New(),
		diag.NewRandom(),
		diag.NewRepeat(),
		diag.NewRepeatHex(),
		diag.NewWrap(),
		diag.NewWrapHex(),
		diag.NewLatency(),
		diag.NewBandwidth(),
		diag.NewChunks(),
		diag.NewBackendLatency(),
		diag.NewBackendBandwidth(),
		diag.NewBackendChunks(),
		diag.NewTarpit(),
		diag.NewAbsorb(),
		diag.NewAbsorbSilent(),
		diag.NewLogHeader(),
		diag.NewLogBody(),
		diag.NewUniformRequestLatency(),
		diag.NewUniformResponseLatency(),
		diag.NewNormalRequestLatency(),
		diag.NewNormalResponseLatency(),
		diag.NewHistogramRequestLatency(),
		diag.NewHistogramResponseLatency(),
		tee.NewTee(),
		tee.NewTeeDeprecated(),
		tee.NewTeeNoFollow(),
		tee.NewTeeLoopback(),
		sed.New(),
		sed.NewDelimited(),
		sed.NewRequest(),
		sed.NewDelimitedRequest(),
		auth.NewBasicAuth(),
		cookie.NewDropRequestCookie(),
		cookie.NewDropResponseCookie(),
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
		tracing.NewTagFromResponse(),
		tracing.NewTagFromResponseIfStatus(),
		tracing.NewStateBagToTag(),
		//lint:ignore SA1019 due to backward compatibility
		accesslog.NewAccessLogDisabled(),
		accesslog.NewDisableAccessLog(),
		accesslog.NewMaskAccessLogQuery(),
		accesslog.NewEnableAccessLog(),
		auth.NewForwardToken(),
		auth.NewForwardTokenField(),
		scheduler.NewFifo(),
		scheduler.NewFifoWithBody(),
		scheduler.NewLIFO(),
		scheduler.NewLIFOGroup(),
		rfc.NewPath(),
		rfc.NewHost(),
		fadein.NewFadeIn(),
		fadein.NewEndpointCreated(),
		consistenthash.NewConsistentHashKey(),
		consistenthash.NewConsistentHashBalanceFactor(),
		tls.New(),
	}
}

// MakeRegistry returns a Registry object initialized with the default
// set of filter specifications found in the filters
// package. (including the builtin and the flowid subdirectories.)
func MakeRegistry() filters.Registry {
	r := make(filters.Registry)
	for _, s := range Filters() {
		r.Register(s)
	}
	return r
}
