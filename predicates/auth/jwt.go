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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	authHeaderName   = "Authorization"
	authHeaderPrefix = "Bearer "
)

type (
	matchBehavior int
)

const (
	matchJWTPayloadAllKVName = "JWTPayloadAllKV"
	matchJWTPayloadAnyKVName = "JWTPayloadAnyKV"

	matchBehaviorAll matchBehavior = iota
	matchBehaviorAny
)

type (
	spec struct {
		matchBehavior matchBehavior
		name          string
	}
	predicate struct {
		kv            map[string][]string
		matchBehavior matchBehavior
	}
)

func NewJWTPayloadAnyKV() routing.PredicateSpec {
	return &spec{
		matchBehavior: matchBehaviorAny,
		name:          matchJWTPayloadAnyKVName,
	}
}

func NewJWTPayloadAllKV() routing.PredicateSpec {
	return &spec{
		matchBehavior: matchBehaviorAll,
		name:          matchJWTPayloadAllKVName,
	}
}

func (s *spec) Name() string {
	return s.name
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	kv := make(map[string][]string)
	for i := 0; i < len(args); i += 2 {
		key, keyOk := args[i].(string)
		value, valueOk := args[i+1].(string)
		if !keyOk || !valueOk {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		kv[key] = append(kv[key], value)
	}

	return &predicate{
		kv:            kv,
		matchBehavior: s.matchBehavior,
	}, nil
}

func (p *predicate) Match(r *http.Request) bool {
	ahead := r.Header.Get(authHeaderName)
	if !strings.HasPrefix(ahead, authHeaderPrefix) {
		return false
	}

	fields := strings.FieldsFunc(ahead, func(r rune) bool {
		return r == []rune(".")[0]
	})
	if len(fields) != 3 {
		return false
	}

	sDec, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return false
	}

	var payload map[string]interface{}
	err = json.Unmarshal(sDec, &payload)
	if err != nil {
		return false
	}

	switch p.matchBehavior {
	case matchBehaviorAll:
		return allMatch(p.kv, payload)
	case matchBehaviorAny:
		return anyMatch(p.kv, payload)
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

func allMatch(expected map[string][]string, payload map[string]interface{}) bool {
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
			if expectedValue != payloadValue {
				return false
			}
		}
	}
	return true
}

func anyMatch(expected map[string][]string, payload map[string]interface{}) bool {
	if len(expected) == 0 {
		return true
	}
	for key, expectedValues := range expected {
		if payloadValue, ok := stringValue(payload, key); ok {
			for _, expectedValue := range expectedValues {
				if expectedValue == payloadValue {
					return true
				}
			}
		}
	}
	return false
}
