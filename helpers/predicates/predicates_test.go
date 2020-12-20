package predicates

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/helpers"
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
	"github.com/zalando/skipper/routing"
	"net/http"
	"regexp"
	"testing"
	"time"
)

func TestArgumentConversion(t *testing.T) {
	methodPredicate := Methods(http.MethodGet, http.MethodPost)
	if len(methodPredicate.Args) != 2 {
		t.Errorf("expected 2 arguments in Methods predicate, got %d", len(methodPredicate.Args))
	}

	kvPredicate := JWTPayloadAllKV(helpers.NewKVPair("k1", "v1"), helpers.NewKVPair("k2", "v2"))
	if len(kvPredicate.Args) != 4 {
		t.Errorf("expected 4 arguments in JWTPayloadAllKV predicate, got %d", len(kvPredicate.Args))
	}

	r := regexp.MustCompile(`/\d+/`)
	kvRegexPredicate := JWTPayloadAllKVRegexp(helpers.NewKVRegexPair("k1", r), helpers.NewKVRegexPair("k2", r))
	if len(kvRegexPredicate.Args) != 4 {
		t.Errorf("expected 4 arguments in JWTPayloadAllKV predicate, got %d", len(kvRegexPredicate.Args))
	}
}

func TestPredicateCreation(t *testing.T) {
	regex := regexp.MustCompile("/[a-z]+/")
	kvPair := helpers.NewKVPair("iss", "https://accounts.google.com")
	kvRegexPair := helpers.NewKVRegexPair("iss", regex)

	t.Run("Path()", testPredicatesWithoutSpec(Path("/skipper")))
	t.Run("PathSubtree()", testPredicatesWithoutSpec(PathSubtree("/skipper")))
	t.Run("PathRegexp()", testPredicatesWithoutSpec(PathRegexp(regex)))
	t.Run("Host()", testPredicatesWithoutSpec(Host(regex)))
	t.Run("Weight()", testPredicatesWithoutSpec(Weight(42)))
	t.Run("True()", testWithSpecFn(primitive.NewTrue(), True()))
	t.Run("False()", testWithSpecFn(primitive.NewFalse(), False()))
	t.Run("Method()", testPredicatesWithoutSpec(Method("test blalalalaal")))
	t.Run("Methods()", testWithSpecFn(methods.New(), Methods(http.MethodGet, http.MethodPost)))
	t.Run("Header()", testPredicatesWithoutSpec(Header("key", "value")))
	t.Run("HeaderRegexp()", testPredicatesWithoutSpec(HeaderRegexp("key", regex)))
	t.Run("Cookie()", testWithSpecFn(cookie.New(), Cookie("cookieName", regex)))
	t.Run("JWTPayloadAnyKV(single arg)", testWithSpecFn(auth.NewJWTPayloadAnyKV(), JWTPayloadAnyKV(kvPair)))
	t.Run("JWTPayloadAnyKV(multiple args)", testWithSpecFn(auth.NewJWTPayloadAnyKV(), JWTPayloadAnyKV(kvPair, kvPair)))
	t.Run("JWTPayloadAllKV(single arg)", testWithSpecFn(auth.NewJWTPayloadAllKV(), JWTPayloadAllKV(kvPair)))
	t.Run("JWTPayloadAllKV(multiple args)", testWithSpecFn(auth.NewJWTPayloadAllKV(), JWTPayloadAllKV(kvPair, kvPair)))
	t.Run("JWTPayloadAnyKVRegexp(single arg)", testWithSpecFn(auth.NewJWTPayloadAnyKVRegexp(), JWTPayloadAnyKVRegexp(kvRegexPair)))
	t.Run("JWTPayloadAnyKVRegexp(multiple args)", testWithSpecFn(auth.NewJWTPayloadAnyKVRegexp(), JWTPayloadAnyKVRegexp(kvRegexPair, kvRegexPair)))
	t.Run("JWTPayloadAllKVRegexp(single arg)", testWithSpecFn(auth.NewJWTPayloadAllKVRegexp(), JWTPayloadAllKVRegexp(kvRegexPair)))
	t.Run("JWTPayloadAllKVRegexp(multiple args)", testWithSpecFn(auth.NewJWTPayloadAllKVRegexp(), JWTPayloadAllKVRegexp(kvRegexPair, kvRegexPair)))
	t.Run("After()", testWithSpecFn(interval.NewAfter(), After(time.Now())))
	t.Run("AfterWithDateString()", testWithSpecFn(interval.NewAfter(), AfterWithDateString("2020-12-19T00:00:00+00:00")))
	t.Run("AfterWithUnixTime()", testWithSpecFn(interval.NewAfter(), AfterWithUnixTime(time.Now().Unix())))
	t.Run("Before()", testWithSpecFn(interval.NewBefore(), Before(time.Now())))
	t.Run("BeforeWithDateString()", testWithSpecFn(interval.NewBefore(), BeforeWithDateString("2020-12-19T00:00:00+00:00")))
	t.Run("BeforeWithUnixTime()", testWithSpecFn(interval.NewBefore(), BeforeWithUnixTime(time.Now().Unix())))
	t.Run("Between()", testWithSpecFn(interval.NewBetween(), Between(time.Now(), time.Now().Add(time.Hour))))
	t.Run("BetweenWithDateString()", testWithSpecFn(interval.NewBetween(), BetweenWithDateString("2020-12-19T00:00:00+00:00", "2020-12-19T01:00:00+00:00")))
	t.Run("BetweenWithUnixTime()", testWithSpecFn(interval.NewBetween(), BetweenWithUnixTime(time.Now().Unix(), time.Now().Add(time.Hour).Unix())))
	t.Run("Cron()", testWithSpecFn(cron.New(), Cron("* * * * *")))
	t.Run("QueryParam()", testWithSpecFn(query.New(), QueryParam("skipper")))
	t.Run("QueryParamWithValueRegex()", testWithSpecFn(query.New(), QueryParamWithValueRegex("skipper", regex)))
	t.Run("Source(single arg)", testWithSpecFn(source.New(), Source("127.0.0.1")))
	t.Run("Source(multiple args)", testWithSpecFn(source.New(), Source("127.0.0.1", "10.0.0.0/24")))
	t.Run("SourceFromLast(single arg)", testWithSpecFn(source.NewFromLast(), SourceFromLast("127.0.0.1")))
	t.Run("SourceFromLast(multiple args)", testWithSpecFn(source.NewFromLast(), SourceFromLast("127.0.0.1", "10.0.0.0/24")))
	t.Run("ClientIP(single arg)", testWithSpecFn(source.NewClientIP(), ClientIP("127.0.0.1")))
	t.Run("ClientIP(multiple args)", testWithSpecFn(source.NewClientIP(), ClientIP("127.0.0.1", "10.0.0.0/24")))
	t.Run("Tee()", testWithSpecFn(tee.New(), Tee("skipper")))
	t.Run("Traffic()", testWithSpecFn(traffic.New(), Traffic(.25)))
	t.Run("TrafficSticky()", testWithSpecFn(traffic.New(), TrafficSticky(.25, "catalog-test", "A")))
}

func testWithSpecFn(predicateSpec routing.PredicateSpec, predicate *eskip.Predicate) func(t *testing.T) {
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
		case routing.HostRegexpName:
			_, err = core.ValidateHostRegexpPredicate(predicate)
		case routing.PathRegexpName:
			_, err = core.ValidatePathRegexpPredicate(predicate)
		case routing.MethodName:
			_, err = core.ValidateMethodPredicate(predicate)
		case routing.HeaderName:
			_, err = core.ValidateHeaderPredicate(predicate)
		case routing.HeaderRegexpName:
			_, err = core.ValidateHeaderRegexpPredicate(predicate)
		case routing.WeightPredicateName:
			_, err = weight.ParseWeightPredicateArgs(predicate.Args)
		case routing.PathName, routing.PathSubtreeName:
			_, err = core.ProcessPathOrSubTree(predicate)
		default:
			t.Errorf("Unknown predicate provided %q", predicate.Name)
		}
		if err != nil {
			t.Errorf("unexpected error while parsing %s predicate with args %s, %v", predicate.Name, predicate.Args, err)
		}
	}
}
