/*
Package auth implements custom predicates to match based on content
of the HTTP Authorization header.

This predicate can be used to match a route based on data in the 2nd
part of a JWT token, for example based on the issuer.

Examples:

    // one key value pair has to match
    example1: JWTPayloadAnyKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
	-> "http://example.org/";
    // all key value pairs have to match
    example2: * && JWTPayloadAllKV("iss", "https://accounts.google.com", "email", "skipper-router@googlegroups.com")
	-> "http://example.org/";
*/
package auth

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/zalando/skipper/jwt"
	"github.com/zalando/skipper/predicates"
)

const (
	authHeaderName   = "Authorization"
	authHeaderPrefix = "Bearer "
)

type (
	matchBehavior int
	matchMode     int
)

type valueMatcher interface {
	Match(jwtValue string) bool
}

const (
	// MatchJWTPayloadAllKVName represents the name of the builtin JWT Payload all Key/Value predicate.

	// MatchJWTPayloadAnyKVName represents the name of the builtin JWT Payload any Key/Value predicate.

	// MatchJWTPayloadAllKVRegexpName represents the name of the builtin JWT Payload all Key/Value regexp predicate.

	// MatchJWTPayloadAnyKVRegexpName represents the name of the builtin JWT Payload any Key/Value regexp predicate.

	matchBehaviorAll matchBehavior = iota
	matchBehaviorAny

	matchModeExact matchMode = iota
	matchModeRegexp
)

type (
	spec struct {
		name          string
		matchBehavior matchBehavior
		matchMode     matchMode
	}
	predicate struct {
		kv            map[string][]valueMatcher
		matchBehavior matchBehavior
	}
	exactMatcher struct {
		expected string
	}
	regexMatcher struct {
		regexp *regexp.Regexp
	}
)

func NewJWTPayloadAnyKV() predicates.PredicateSpec {
	return &spec{
		name:          predicates.MatchJWTPayloadAnyKVName,
		matchBehavior: matchBehaviorAny,
		matchMode:     matchModeExact,
	}
}

func NewJWTPayloadAllKV() predicates.PredicateSpec {
	return &spec{
		name:          predicates.MatchJWTPayloadAllKVName,
		matchBehavior: matchBehaviorAll,
		matchMode:     matchModeExact,
	}
}

func NewJWTPayloadAnyKVRegexp() predicates.PredicateSpec {
	return &spec{
		name:          predicates.MatchJWTPayloadAnyKVRegexpName,
		matchBehavior: matchBehaviorAny,
		matchMode:     matchModeRegexp,
	}
}

func NewJWTPayloadAllKVRegexp() predicates.PredicateSpec {
	return &spec{
		name:          predicates.MatchJWTPayloadAllKVRegexpName,
		matchBehavior: matchBehaviorAll,
		matchMode:     matchModeRegexp,
	}
}

func (s *spec) Name() string {
	return s.name
}

func (s *spec) Create(args []interface{}) (predicates.Predicate, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	kv := make(map[string][]valueMatcher)
	for i := 0; i < len(args); i += 2 {
		key, keyOk := args[i].(string)
		value, valueOk := args[i+1].(string)
		if !keyOk || !valueOk {
			return nil, predicates.ErrInvalidPredicateParameters
		}

		var matcher valueMatcher
		switch s.matchMode {
		case matchModeExact:
			matcher = exactMatcher{expected: value}
		case matchModeRegexp:
			re, err := regexp.Compile(value)
			if err != nil {
				return nil, predicates.ErrInvalidPredicateParameters
			}
			matcher = regexMatcher{regexp: re}
		default:
			return nil, predicates.ErrInvalidPredicateParameters
		}
		kv[key] = append(kv[key], matcher)
	}

	return &predicate{
		kv:            kv,
		matchBehavior: s.matchBehavior,
	}, nil
}

func (m exactMatcher) Match(jwtValue string) bool {
	return jwtValue == m.expected
}

func (m regexMatcher) Match(jwtValue string) bool {
	return m.regexp.MatchString(jwtValue)
}

func (p *predicate) Match(r *http.Request) bool {
	ahead := r.Header.Get(authHeaderName)
	tv := strings.TrimPrefix(ahead, authHeaderPrefix)
	if tv == ahead {
		return false
	}

	token, err := jwt.Parse(tv)
	if err != nil {
		return false
	}

	switch p.matchBehavior {
	case matchBehaviorAll:
		return allMatch(p.kv, token.Claims)
	case matchBehaviorAny:
		return anyMatch(p.kv, token.Claims)
	default:
		return false
	}
}

func stringValue(payload map[string]interface{}, key string) (string, bool) {
	if value, ok := payload[key]; ok {
		result, ok := value.(string)
		return result, ok
	}
	return "", false
}

func allMatch(expected map[string][]valueMatcher, payload map[string]interface{}) bool {
	if len(expected) > len(payload) {
		return false
	}
	for key, expectedValues := range expected {
		payloadValue, ok := stringValue(payload, key)
		if !ok {
			return false
		}

		// expectedValues is expected to be a slice of one value
		for _, expectedValue := range expectedValues {
			if !expectedValue.Match(payloadValue) {
				return false
			}
		}
	}
	return true
}

func anyMatch(expected map[string][]valueMatcher, payload map[string]interface{}) bool {
	if len(expected) == 0 {
		return true
	}
	for key, expectedValues := range expected {
		if payloadValue, ok := stringValue(payload, key); ok {
			for _, expectedValue := range expectedValues {
				if expectedValue.Match(payloadValue) {
					return true
				}
			}
		}
	}
	return false
}
