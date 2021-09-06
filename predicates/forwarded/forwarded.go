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
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
	"net/http"
	"regexp"
	"strings"
)

const (
	NameHost  = "ForwardedHost"
	NameProto = "ForwardedProtocol"
)

type hostPredicateSpec struct{}

type protoPredicateSpec struct{}

type hostPredicate struct {
	host *regexp.Regexp
}

func (p *hostPredicateSpec) Create(args []interface{}) (routing.Predicate, error) {
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

func (p *protoPredicateSpec) Create(args []interface{}) (routing.Predicate, error) {

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
	return NameHost
}

func (p *protoPredicateSpec) Name() string {
	return NameProto
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

	for _, forwardedFull := range strings.Split(fh, ",") {
		for _, forwardedPair := range strings.Split(forwardedFull, ";") {
			if tv := strings.SplitN(forwardedPair, "=", 2); len(tv) == 2 {
				token, value := tv[0], tv[1]
				value = strings.Trim(value, `"`)
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
