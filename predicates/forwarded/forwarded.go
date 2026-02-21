/*
Package forwarded implements a set of custom predicate to match routes
based on the standardized Forwarded header.

https://datatracker.ietf.org/doc/html/rfc7239
https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Forwarded

Examples:

	// only match requests to "example.com"
	example1: ForwardedHost("example.com") -> "http://example.org";

	// only match requests to http
	example2: ForwardedProtocol("http") -> "http://example.org";

	// only match requests to https
	example3: ForwardedProtocol("https") -> "http://example.org";
*/
package forwarded

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

const (
	// Deprecated, use predicates.ForwardedHostName instead
	NameHost = predicates.ForwardedHostName
	// Deprecated, use predicates.ForwardedProtocolName instead
	NameProto = predicates.ForwardedProtocolName
)

type hostPredicateSpec struct{}

type protoPredicateSpec struct{}

type hostPredicate struct {
	host *regexp.Regexp
}

func (p *hostPredicateSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	value, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if value == "" {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	re, err := regexp.Compile(value)
	if err != nil {
		return nil, err
	}

	return hostPredicate{host: re}, err
}

type protoPredicate struct {
	proto string
}

func (p *protoPredicateSpec) Create(args []any) (routing.Predicate, error) {

	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	value, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	switch value {
	case "http", "https":
		return protoPredicate{proto: value}, nil
	default:
		return nil, predicates.ErrInvalidPredicateParameters
	}
}

func NewForwardedHost() routing.PredicateSpec  { return &hostPredicateSpec{} }
func NewForwardedProto() routing.PredicateSpec { return &protoPredicateSpec{} }

func (p *hostPredicateSpec) Name() string {
	return predicates.ForwardedHostName
}

func (p *protoPredicateSpec) Name() string {
	return predicates.ForwardedProtocolName
}

func (p hostPredicate) Match(r *http.Request) bool {

	fh := r.Header.Get("Forwarded")

	if fh == "" {
		return false
	}

	fw := parseForwarded(fh)

	return p.host.MatchString(fw.host)
}

func (p protoPredicate) Match(r *http.Request) bool {

	fh := r.Header.Get("Forwarded")

	if fh == "" {
		return false
	}

	fw := parseForwarded(fh)

	return p.proto == fw.proto
}

type forwarded struct {
	host  string
	proto string
}

func parseForwarded(fh string) *forwarded {
	f := &forwarded{}

	for forwardedFull := range splitSeq(fh, ",") {
		for forwardedPair := range splitSeq(strings.TrimSpace(forwardedFull), ";") {
			token, value, found := strings.Cut(forwardedPair, "=")
			value = strings.Trim(value, `"`)
			if found && value != "" {
				switch token {
				case "proto":
					f.proto = value
				case "host":
					f.host = value
				}
			}
		}
	}
	return f
}

// TODO: use [strings.SplitSeq] added in go1.24 once go1.25 is released.
func splitSeq(s string, sep string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for {
			i := strings.Index(s, sep)
			if i < 0 {
				break
			}
			frag := s[:i]
			if !yield(frag) {
				return
			}
			s = s[i+len(sep):]
		}
		yield(s)
	}
}
