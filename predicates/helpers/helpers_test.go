package helpers

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/helpers"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/predicates/auth"
	"github.com/zalando/skipper/predicates/cookie"
	"github.com/zalando/skipper/predicates/core"
	"github.com/zalando/skipper/predicates/cron"
	"github.com/zalando/skipper/predicates/interval"
	"github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/predicates/source"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/predicates/weight"
	"net/http"
	"regexp"
	"testing"
	"time"
)

func TestArgumentConversion(t *testing.T) {
	methodPredicate := predicates.Methods(http.MethodGet, http.MethodPost)
	if len(methodPredicate.Args) != 2 {
		t.Errorf("expected 2 arguments in Methods predicate, got %d", len(methodPredicate.Args))
	}

	kvPredicate := predicates.JWTPayloadAllKV(helpers.NewKVPair("k1", "v1"), helpers.NewKVPair("k2", "v2"))
	if len(kvPredicate.Args) != 4 {
		t.Errorf("expected 4 arguments in JWTPayloadAllKV predicate, got %d", len(kvPredicate.Args))
	}

	r := regexp.MustCompile(`/\d+/`)
	kvRegexPredicate := predicates.JWTPayloadAllKVRegexp(helpers.NewKVRegexPair("k1", r), helpers.NewKVRegexPair("k2", r))
	if len(kvRegexPredicate.Args) != 4 {
		t.Errorf("expected 4 arguments in JWTPayloadAllKV predicate, got %d", len(kvRegexPredicate.Args))
	}
}

func TestPredicateCreation(t *testing.T) {
	regex := regexp.MustCompile("/[a-z]+/")
	kvPair := helpers.NewKVPair("iss", "https://accounts.google.com")
	kvRegexPair := helpers.NewKVRegexPair("iss", regex)

	t.Run("Path()", testPredicatesWithoutSpec(predicates.Path("/skipper")))
	t.Run("PathSubtree()", testPredicatesWithoutSpec(predicates.PathSubtree("/skipper")))
	t.Run("PathRegexp()", testPredicatesWithoutSpec(predicates.PathRegexp(regex)))
	t.Run("Host()", testPredicatesWithoutSpec(predicates.Host(regex)))
	t.Run("Weight()", testPredicatesWithoutSpec(predicates.Weight(42)))
	t.Run("True()", testWithSpecFn(primitive.NewTrue(), predicates.True()))
	t.Run("False()", testWithSpecFn(primitive.NewFalse(), predicates.False()))
	t.Run("Method()", testPredicatesWithoutSpec(predicates.Method("test blalalalaal")))
	t.Run("Methods()", testWithSpecFn(methods.New(), predicates.Methods(http.MethodGet, http.MethodPost)))
	t.Run("Header()", testPredicatesWithoutSpec(predicates.Header("key", "value")))
	t.Run("HeaderRegexp()", testPredicatesWithoutSpec(predicates.HeaderRegexp("key", regex)))
	t.Run("Cookie()", testWithSpecFn(cookie.New(), predicates.Cookie("cookieName", regex)))
	t.Run("JWTPayloadAnyKV(single arg)", testWithSpecFn(auth.NewJWTPayloadAnyKV(), predicates.JWTPayloadAnyKV(kvPair)))
	t.Run("JWTPayloadAnyKV(multiple args)", testWithSpecFn(auth.NewJWTPayloadAnyKV(), predicates.JWTPayloadAnyKV(kvPair, kvPair)))
	t.Run("JWTPayloadAllKV(single arg)", testWithSpecFn(auth.NewJWTPayloadAllKV(), predicates.JWTPayloadAllKV(kvPair)))
	t.Run("JWTPayloadAllKV(multiple args)", testWithSpecFn(auth.NewJWTPayloadAllKV(), predicates.JWTPayloadAllKV(kvPair, kvPair)))
	t.Run("JWTPayloadAnyKVRegexp(single arg)", testWithSpecFn(auth.NewJWTPayloadAnyKVRegexp(), predicates.JWTPayloadAnyKVRegexp(kvRegexPair)))
	t.Run("JWTPayloadAnyKVRegexp(multiple args)", testWithSpecFn(auth.NewJWTPayloadAnyKVRegexp(), predicates.JWTPayloadAnyKVRegexp(kvRegexPair, kvRegexPair)))
	t.Run("JWTPayloadAllKVRegexp(single arg)", testWithSpecFn(auth.NewJWTPayloadAllKVRegexp(), predicates.JWTPayloadAllKVRegexp(kvRegexPair)))
	t.Run("JWTPayloadAllKVRegexp(multiple args)", testWithSpecFn(auth.NewJWTPayloadAllKVRegexp(), predicates.JWTPayloadAllKVRegexp(kvRegexPair, kvRegexPair)))
	t.Run("After()", testWithSpecFn(interval.NewAfter(), predicates.After(time.Now())))
	t.Run("AfterWithDateString()", testWithSpecFn(interval.NewAfter(), predicates.AfterWithDateString("2020-12-19T00:00:00+00:00")))
	t.Run("AfterWithUnixTime()", testWithSpecFn(interval.NewAfter(), predicates.AfterWithUnixTime(time.Now().Unix())))
	t.Run("Before()", testWithSpecFn(interval.NewBefore(), predicates.Before(time.Now())))
	t.Run("BeforeWithDateString()", testWithSpecFn(interval.NewBefore(), predicates.BeforeWithDateString("2020-12-19T00:00:00+00:00")))
	t.Run("BeforeWithUnixTime()", testWithSpecFn(interval.NewBefore(), predicates.BeforeWithUnixTime(time.Now().Unix())))
	t.Run("Between()", testWithSpecFn(interval.NewBetween(), predicates.Between(time.Now(), time.Now().Add(time.Hour))))
	t.Run("BetweenWithDateString()", testWithSpecFn(interval.NewBetween(), predicates.BetweenWithDateString("2020-12-19T00:00:00+00:00", "2020-12-19T01:00:00+00:00")))
	t.Run("BetweenWithUnixTime()", testWithSpecFn(interval.NewBetween(), predicates.BetweenWithUnixTime(time.Now().Unix(), time.Now().Add(time.Hour).Unix())))
	t.Run("Cron()", testWithSpecFn(cron.New(), predicates.Cron("* * * * *")))
	t.Run("QueryParam()", testWithSpecFn(query.New(), predicates.QueryParam("skipper")))
	t.Run("QueryParamWithValueRegex()", testWithSpecFn(query.New(), predicates.QueryParamWithValueRegex("skipper", regex)))
	t.Run("Source(single arg)", testWithSpecFn(source.New(), predicates.Source("127.0.0.1")))
	t.Run("Source(multiple args)", testWithSpecFn(source.New(), predicates.Source("127.0.0.1", "10.0.0.0/24")))
	t.Run("SourceFromLast(single arg)", testWithSpecFn(source.NewFromLast(), predicates.SourceFromLast("127.0.0.1")))
	t.Run("SourceFromLast(multiple args)", testWithSpecFn(source.NewFromLast(), predicates.SourceFromLast("127.0.0.1", "10.0.0.0/24")))
	t.Run("ClientIP(single arg)", testWithSpecFn(source.NewClientIP(), predicates.ClientIP("127.0.0.1")))
	t.Run("ClientIP(multiple args)", testWithSpecFn(source.NewClientIP(), predicates.ClientIP("127.0.0.1", "10.0.0.0/24")))
	t.Run("Tee()", testWithSpecFn(tee.New(), predicates.Tee("skipper")))
	t.Run("Traffic()", testWithSpecFn(traffic.New(), predicates.Traffic(.25)))
	t.Run("TrafficSticky()", testWithSpecFn(traffic.New(), predicates.TrafficSticky(.25, "catalog-test", "A")))
}

func testWithSpecFn(predicateSpec predicates.PredicateSpec, predicate *eskip.Predicate) func(t *testing.T) {
	return func(t *testing.T) {
		if predicateSpec.Name() != predicate.Name {
			t.Errorf("spec name and predicate name differ, spec=%s, predicate=%s", predicateSpec.Name(), predicate.Name)
		}
		_, err := predicateSpec.Create(predicate.Args)
		if err != nil {
			t.Errorf("unexpected error while parsing %s predicate with args %s, %v", predicate.Name, predicate.Args, err)
		}
	}
}

func testPredicatesWithoutSpec(predicate *eskip.Predicate) func(t *testing.T) {
	return func(t *testing.T) {
		var err error
		switch predicate.Name {
		case predicates.HostRegexpName:
			_, err = core.ValidateHostRegexpPredicate(predicate)
		case predicates.PathRegexpName:
			_, err = core.ValidatePathRegexpPredicate(predicate)
		case predicates.MethodName:
			_, err = core.ValidateMethodPredicate(predicate)
		case predicates.HeaderName:
			_, err = core.ValidateHeaderPredicate(predicate)
		case predicates.HeaderRegexpName:
			_, err = core.ValidateHeaderRegexpPredicate(predicate)
		case predicates.WeightName:
			_, err = weight.ParseWeightPredicateArgs(predicate.Args)
		case predicates.PathName, predicates.PathSubtreeName:
			_, err = core.ProcessPathOrSubTree(predicate)
		default:
			t.Errorf("Unknown predicate provided %q", predicate.Name)
		}
		if err != nil {
			t.Errorf("unexpected error while parsing %s predicate with args %s, %v", predicate.Name, predicate.Args, err)
		}
	}
}
