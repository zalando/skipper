package predicates

import (
	"errors"
	"net/http"
)

// ErrInvalidPredicateParameters is used in case of invalid predicate parameters.
var ErrInvalidPredicateParameters = errors.New("invalid predicate parameters")

// legacy, non-tree predicate names:
const (
	HostRegexpName   = "Host"
	PathRegexpName   = "PathRegexp"
	MethodName       = "Method"
	HeaderName       = "Header"
	HeaderRegexpName = "HeaderRegexp"
)
const (
	// PathName represents the name of builtin path predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://godoc.org/github.com/zalando/skipper/eskip)
	PathName = "Path"
	// PathSubtreeName represents the name of the builtin path subtree predicate.
	// (See more details about the Path and PathSubtree predicates
	// at https://godoc.org/github.com/zalando/skipper/eskip)
	PathSubtreeName                = "PathSubtree"
	WeightName                     = "Weight"
	TrueName                       = "True"
	FalseName                      = "False"
	MethodsName                    = "Methods"
	CookieName                     = "Cookie"
	MatchJWTPayloadAllKVName       = "JWTPayloadAllKV"
	MatchJWTPayloadAnyKVName       = "JWTPayloadAnyKV"
	MatchJWTPayloadAllKVRegexpName = "JWTPayloadAllKVRegexp"
	MatchJWTPayloadAnyKVRegexpName = "JWTPayloadAnyKVRegexp"
	BetweenName                    = "Between"
	BeforeName                     = "Before"
	AfterName                      = "After"
	CronName                       = "Cron"
	QueryParamName                 = "QueryParam"
	SourceName                     = "Source"
	SourceFromLastName             = "SourceFromLast"
	ClientIPName                   = "ClientIP"
	TeeName                        = "Tee"
	TrafficName                    = "Traffic"
)

// PredicateSpec instances are used to create custom predicates
// (of type Predicate) with concrete arguments during the
// construction of the routing tree.
type PredicateSpec interface {

	// Name of the predicate as used in the route definitions.
	Name() string

	// Creates a predicate instance with concrete arguments.
	Create([]interface{}) (Predicate, error)
}

// Predicate instances are used as custom user defined route
// matching predicates.
type Predicate interface {

	// Returns true if the request matches the predicate.
	Match(*http.Request) bool
}
