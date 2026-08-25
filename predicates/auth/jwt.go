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
	"sync"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/jwt"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
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
	matchBehaviorAll matchBehavior = iota
	matchBehaviorAny

	matchModeExact matchMode = iota
	matchModeRegexp
)

type (
	registry struct {
		quit chan struct{}

		// Map to share predicate instance for the same config
		// and do not create waste on each route table update
		mu           sync.Mutex
		predicateMap map[string]*predicate // eskip string to predicate
	}

	spec struct {
		name          string
		matchBehavior matchBehavior
		matchMode     matchMode
		reg           *registry
	}
	predicate struct {
		kv            map[string][]valueMatcher
		matchBehavior matchBehavior
		cache         sync.Map
	}
	exactMatcher struct {
		expected string
	}
	regexMatcher struct {
		regexp *regexp.Regexp
	}
)

func (r *registry) Close() {
	close(r.quit)
}

func (r *registry) clean() {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()

	for {
		select {
		case <-r.quit:
			return
		case <-tick.C:
			r.mu.Lock()
			for _, p := range r.predicateMap {
				p.cache.Clear()
			}
			r.mu.Unlock()
		}
	}
}

func NewJWTPayloadAnyKV() routing.PredicateSpec {
	return &spec{
		name:          predicates.JWTPayloadAnyKVName,
		matchBehavior: matchBehaviorAny,
		matchMode:     matchModeExact,
	}
}

func NewJWTPayloadAllKV() routing.PredicateSpec {
	return &spec{
		name:          predicates.JWTPayloadAllKVName,
		matchBehavior: matchBehaviorAll,
		matchMode:     matchModeExact,
	}
}

func NewJWTPayloadAnyKVRegexp() routing.PredicateSpec {
	reg := &registry{
		quit:         make(chan struct{}),
		predicateMap: make(map[string]*predicate),
	}
	go reg.clean()

	return &spec{
		name:          predicates.JWTPayloadAnyKVRegexpName,
		matchBehavior: matchBehaviorAny,
		matchMode:     matchModeRegexp,
		reg:           reg,
	}
}

func NewJWTPayloadAllKVRegexp() routing.PredicateSpec {
	reg := &registry{
		quit:         make(chan struct{}),
		predicateMap: make(map[string]*predicate),
	}
	go reg.clean()

	return &spec{
		name:          predicates.JWTPayloadAllKVRegexpName,
		matchBehavior: matchBehaviorAll,
		matchMode:     matchModeRegexp,
		reg:           reg,
	}
}

func (s *spec) Name() string {
	return s.name
}

func (s *spec) Create(args []any) (routing.Predicate, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	// lookup cached predicate
	if s.matchMode == matchModeRegexp {
		sp := (&eskip.Predicate{
			Name: s.Name(),
			Args: args,
		}).String()
		s.reg.mu.Lock()
		if p, ok := s.reg.predicateMap[sp]; ok {
			s.reg.mu.Unlock()
			return p, nil
		}
		s.reg.mu.Unlock()
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

	p := &predicate{
		kv:            kv,
		matchBehavior: s.matchBehavior,
	}

	// store predicate to cache
	if s.matchMode == matchModeRegexp {
		sp := (&eskip.Predicate{
			Name: s.Name(),
			Args: args,
		}).String()
		s.reg.mu.Lock()
		s.reg.predicateMap[sp] = p
		s.reg.mu.Unlock()
	}

	return p, nil
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

	if v, ok := p.cache.Load(ahead); ok {
		return v.(bool)
	}

	token, err := jwt.Parse(tv)
	if err != nil {
		p.cache.Store(ahead, false)
		return false
	}

	var res bool
	switch p.matchBehavior {
	case matchBehaviorAll:
		res = allMatch(p.kv, token.Claims)
	case matchBehaviorAny:
		res = anyMatch(p.kv, token.Claims)
	default:
		res = false
	}
	p.cache.Store(ahead, res)
	return res
}

func stringValue(payload map[string]any, key string) (string, bool) {
	if value, ok := payload[key]; ok {
		result, ok := value.(string)
		return result, ok
	}
	return "", false
}

func allMatch(expected map[string][]valueMatcher, payload map[string]any) bool {
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

func anyMatch(expected map[string][]valueMatcher, payload map[string]any) bool {
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
