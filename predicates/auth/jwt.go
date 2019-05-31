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

type roleMatchType int

const (
	matchJWTPayloadAnyKV roleMatchType = iota
	matchJWTPayloadAllKV

	matchJWTPayloadAllKVName = "JWTPayloadAllKV"
	matchJWTPayloadAnyKVName = "JWTPayloadAnyKV"
	matchUnkown              = "unkown"
)

type (
	spec         struct{ typ roleMatchType }
	predicateAny struct {
		kv map[string][]string
	}
	predicateAll struct {
		kv map[string]string
	}
)

func NewJWTPayloadAnyKV() routing.PredicateSpec {
	return &spec{typ: matchJWTPayloadAnyKV}
}

func NewJWTPayloadAllKV() routing.PredicateSpec {
	return &spec{typ: matchJWTPayloadAllKV}
}

func (s *spec) Name() string {
	switch s.typ {
	case matchJWTPayloadAllKV:
		return matchJWTPayloadAllKVName
	case matchJWTPayloadAnyKV:
		return matchJWTPayloadAnyKVName
	}
	return matchUnkown
}

func (s *spec) Create(args []interface{}) (routing.Predicate, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	var k string

	switch s.typ {
	case matchJWTPayloadAllKV:
		kv := make(map[string]string)
		for i := range args {
			if s, ok := args[i].(string); ok {
				switch i % 2 {
				case 0:
					k = s
					kv[k] = ""
				case 1:
					kv[k] = s
				}
			} else {
				return nil, predicates.ErrInvalidPredicateParameters
			}
		}
		return &predicateAll{kv: kv}, nil
	case matchJWTPayloadAnyKV:
		kv := make(map[string][]string)
		for i := range args {
			if s, ok := args[i].(string); ok {
				switch i % 2 {
				case 0:
					k = s
					if _, ok := kv[k]; !ok {
						kv[k] = []string{}
					}
				case 1:
					kv[k] = append(kv[k], s)
				}
			} else {
				return nil, predicates.ErrInvalidPredicateParameters
			}
		}
		return &predicateAny{kv: kv}, nil
	}

	return nil, predicates.ErrInvalidPredicateParameters
}

func (p *predicateAll) Match(r *http.Request) bool {
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

	var h map[string]interface{}
	err = json.Unmarshal(sDec, &h)
	if err != nil {
		return false
	}

	return allMatch(p.kv, h)
}

func (p *predicateAny) Match(r *http.Request) bool {
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

	var h map[string]interface{}
	err = json.Unmarshal(sDec, &h)
	if err != nil {
		return false
	}

	return anyMatch(p.kv, h)
}

func allMatch(kv map[string]string, h map[string]interface{}) bool {
	if len(kv) > len(h) {
		return false
	}
	for k, v := range kv {
		if vh, ok := h[k]; !ok {
			return false
		} else {
			if s, ok2 := vh.(string); !ok2 || v != s {
				return false
			}
		}
	}
	return true
}

func anyMatch(kv map[string][]string, h map[string]interface{}) bool {
	if len(kv) == 0 {
		return true
	}
	for k, a := range kv {
		if vh, ok := h[k]; ok {
			var s string
			if s, ok = vh.(string); !ok {
				return false
			}
			for _, v := range a {
				if v == s {
					return true
				}
			}
		}
	}
	return false
}
