package predicates

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/helpers"
	"github.com/zalando/skipper/predicates/auth"
	"github.com/zalando/skipper/predicates/cookie"
	"github.com/zalando/skipper/predicates/cron"
	"github.com/zalando/skipper/predicates/interval"
	methodpredicate "github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/routing"
	"regexp"
	"time"
)

// Path
func Path(path string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.PathName,
		Args: []interface{}{path},
	}
}

// PathSubtree
func PathSubtree(path string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.PathSubtreeName,
		Args: []interface{}{path},
	}
}

// PathRegexp
func PathRegexp(path *regexp.Regexp) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.PathRegexpName,
		Args: []interface{}{path.String()},
	}
}

// Host
func Host(host *regexp.Regexp) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.HostRegexpName,
		Args: []interface{}{host.String()},
	}
}

// Weight (priority)
func Weight(weight int) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.WeightName,
		Args: []interface{}{weight},
	}
}

// True
func True() *eskip.Predicate {
	return &eskip.Predicate{
		Name: primitive.TrueName,
	}
}

// False
func False() *eskip.Predicate {
	return &eskip.Predicate{
		Name: primitive.FalseName,
	}
}

// Method
func Method(method string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.MethodName,
		Args: []interface{}{method},
	}
}

// Methods
func Methods(methods ...string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: methodpredicate.MethodsName,
		Args: stringSliceToArgs(methods),
	}
}

// Header
func Header(key, value string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.HeaderName,
		Args: []interface{}{key, value},
	}
}

// HeaderRegexp
func HeaderRegexp(key string, value *regexp.Regexp) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.HeaderRegexpName,
		Args: []interface{}{key, value.String()},
	}
}

// Cookie
func Cookie(name string, value *regexp.Regexp) *eskip.Predicate {
	return &eskip.Predicate{
		Name: cookie.CookieName,
		Args: []interface{}{name, value.String()},
	}
}

// JWTPayloadAnyKV
func JWTPayloadAnyKV(pairs ...helpers.KVPair) *eskip.Predicate {
	return &eskip.Predicate{
		Name: auth.MatchJWTPayloadAnyKVName,
		Args: helpers.KVPairToArgs(pairs),
	}
}

// JWTPayloadAllKV
func JWTPayloadAllKV(pairs ...helpers.KVPair) *eskip.Predicate {
	return &eskip.Predicate{
		Name: auth.MatchJWTPayloadAllKVName,
		Args: helpers.KVPairToArgs(pairs),
	}
}

// JWTPayloadAnyKVRegexp
func JWTPayloadAnyKVRegexp(pairs ...helpers.KVRegexPair) *eskip.Predicate {
	return &eskip.Predicate{
		Name: auth.MatchJWTPayloadAnyKVRegexpName,
		Args: helpers.KVRegexPairToArgs(pairs),
	}
}

// JWTPayloadAllKVRegexp
func JWTPayloadAllKVRegexp(pairs ...helpers.KVRegexPair) *eskip.Predicate {
	return &eskip.Predicate{
		Name: auth.MatchJWTPayloadAllKVRegexpName,
		Args: helpers.KVRegexPairToArgs(pairs),
	}
}

// After
func After(date time.Time) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.AfterName,
		Args: []interface{}{date.Format(time.RFC3339)},
	}
}

// AfterWithDateString
func AfterWithDateString(date string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.AfterName,
		Args: []interface{}{date},
	}
}

// AfterWithUnixTime
func AfterWithUnixTime(time int64) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.AfterName,
		Args: []interface{}{time},
	}
}

// Before
func Before(date time.Time) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BeforeName,
		Args: []interface{}{date.Format(time.RFC3339)},
	}
}

// BeforeWithDateString
func BeforeWithDateString(date string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BeforeName,
		Args: []interface{}{date},
	}
}

// BeforeWithUnixTime
func BeforeWithUnixTime(time int64) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BeforeName,
		Args: []interface{}{time},
	}
}

// Between
func Between(from, until time.Time) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BetweenName,
		Args: []interface{}{from.Format(time.RFC3339), until.Format(time.RFC3339)},
	}
}

// BetweenWithDateString
func BetweenWithDateString(from, until string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BetweenName,
		Args: []interface{}{from, until},
	}
}

// BetweenWithUnixTime
func BetweenWithUnixTime(from, until int64) *eskip.Predicate {
	return &eskip.Predicate{
		Name: interval.BetweenName,
		Args: []interface{}{from, until},
	}
}

// Cron
func Cron(expression string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: cron.CronName,
		Args: []interface{}{expression},
	}
}

// QueryParam
func QueryParam(name string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: query.QueryParamName,
		Args: []interface{}{name},
	}
}

// QueryParamWithValueRegex
func QueryParamWithValueRegex(name string, value *regexp.Regexp) *eskip.Predicate {
	return &eskip.Predicate{
		Name: query.QueryParamName,
		Args: []interface{}{name, value.String()},
	}
}

// Source
func Source(networkRanges ...string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: source.SourceName,
		Args: stringSliceToArgs(networkRanges),
	}
}

// SourceFromLast
func SourceFromLast(networkRanges ...string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: source.SourceFromLastName,
		Args: stringSliceToArgs(networkRanges),
	}
}

// ClientIP
func ClientIP(networkRanges ...string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: source.ClientIPName,
		Args: stringSliceToArgs(networkRanges),
	}
}

// Tee
func Tee(label string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: tee.TeeName,
		Args: []interface{}{label},
	}
}

// Traffic
func Traffic(chance float64) *eskip.Predicate {
	return &eskip.Predicate{
		Name: traffic.TrafficName,
		Args: []interface{}{chance},
	}
}

// TrafficSticky
func TrafficSticky(chance float64, trafficGroupCookie, trafficGroup string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: traffic.TrafficName,
		Args: []interface{}{chance, trafficGroupCookie, trafficGroup},
	}
}

func stringSliceToArgs(strings []string) []interface{} {
	args := make([]interface{}, 0, len(strings))
	for _, s := range strings {
		args = append(args, s)
	}
	return args
}
