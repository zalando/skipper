// Package predicates provides packages to enhance routing match capabilities.
package predicates

import "errors"

// ErrInvalidPredicateParameters is used in case of invalid predicate parameters.
var ErrInvalidPredicateParameters = errors.New("invalid predicate parameters")

// All Skipper Predicate names
const (
	// PathName represents the name of builtin path predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://pkg.go.dev/github.com/zalando/skipper/eskip)
	PathName = "Path"
	// PathSubtreeName represents the name of the builtin path subtree predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://pkg.go.dev/github.com/zalando/skipper/eskip)
	PathSubtreeName           = "PathSubtree"
	PathRegexpName            = "PathRegexp"
	HostName                  = "Host"
	HostAnyName               = "HostAny"
	ForwardedHostName         = "ForwardedHost"
	ForwardedProtocolName     = "ForwardedProtocol"
	WeightName                = "Weight"
	TrueName                  = "True"
	FalseName                 = "False"
	ShutdownName              = "Shutdown"
	MethodName                = "Method"
	MethodsName               = "Methods"
	HeaderName                = "Header"
	HeaderRegexpName          = "HeaderRegexp"
	CookieName                = "Cookie"
	JWTPayloadAnyKVName       = "JWTPayloadAnyKV"
	JWTPayloadAllKVName       = "JWTPayloadAllKV"
	JWTPayloadAnyKVRegexpName = "JWTPayloadAnyKVRegexp"
	JWTPayloadAllKVRegexpName = "JWTPayloadAllKVRegexp"
	HeaderSHA256Name          = "HeaderSHA256"
	AfterName                 = "After"
	BeforeName                = "Before"
	BetweenName               = "Between"
	CronName                  = "Cron"
	QueryParamName            = "QueryParam"
	SourceName                = "Source"
	SourceFromLastName        = "SourceFromLast"
	ClientIPName              = "ClientIP"
	TeeName                   = "Tee"
	TrafficName               = "Traffic"
	TrafficSegmentName        = "TrafficSegment"
	ContentLengthBetweenName  = "ContentLengthBetween"
	OTelBaggageName           = "OTelBaggage"
)
