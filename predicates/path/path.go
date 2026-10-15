// Package path supports path based predicates that work on the
// request object similar to all non path based predicates.
package path

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

// rawPath returns the raw (URL-encoded) request path when available,
// falling back to the decoded path. It avoids calling URL.EscapedPath()
// which allocates when RawPath is set.
func rawPath(r *http.Request) string {
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	return r.URL.Path
}

type pathPrefixSpec struct{}

type pathPrefixPredicate struct {
	prefix string
}

// NewUrlPathPrefix creates a predicate specification whose instances match
// requests where the decoded path (URL.Path) starts with the given prefix.
//
// Unlike the builtin Path and PathSubtree predicates, UrlPathPrefix belongs
// to the second evaluation group (like PathRegexp) and does not participate
// in the path tree. It can therefore match paths containing double slashes
// that are preserved in URL.Path since Skipper does not use http.ServeMux.
// For paths with percent-encoded characters such as %2F, use UrlRawPathPrefix.
func NewUrlPathPrefix() routing.PredicateSpec { return &pathPrefixSpec{} }

func (*pathPrefixSpec) Name() string { return predicates.UrlPathPrefixName }

func (*pathPrefixSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	prefix, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &pathPrefixPredicate{prefix: prefix}, nil
}

func (p *pathPrefixPredicate) Match(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, p.prefix)
}

type pathRegexpMatchSpec struct{}

type pathRegexpMatchPredicate struct {
	re *regexp.Regexp
}

// NewUrlPathRegexp creates a predicate specification whose instances match
// requests where the decoded path (URL.Path) matches the given regular expression.
//
// Unlike the builtin PathRegexp, this is a plugin predicate and can be combined
// freely with PathRegexp in the same route. Like PathRegexp it belongs to the
// second evaluation group, not the path tree. Double slashes are preserved in
// URL.Path since Skipper does not use http.ServeMux. For percent-encoded
// characters such as %2F, use UrlRawPathRegexp.
func NewUrlPathRegexp() routing.PredicateSpec { return &pathRegexpMatchSpec{} }

func (*pathRegexpMatchSpec) Name() string { return predicates.UrlPathRegexpName }

func (*pathRegexpMatchSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	pattern, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &pathRegexpMatchPredicate{re: re}, nil
}

func (p *pathRegexpMatchPredicate) Match(r *http.Request) bool {
	return p.re.MatchString(r.URL.Path)
}

type rawUrlPathPrefixSpec struct{}

type rawUrlPathPrefixPredicate struct {
	prefix string
}

// NewUrlRawPathPrefix creates a predicate specification whose instances match
// requests where the raw (URL-encoded) path starts with the given prefix.
//
// It reads URL.RawPath when set, falling back to URL.Path. Use this when
// the path may contain percent-encoded reserved characters such as %2F
// (encoded slash) that Go's HTTP server decodes in URL.Path.
func NewUrlRawPathPrefix() routing.PredicateSpec { return &rawUrlPathPrefixSpec{} }

func (*rawUrlPathPrefixSpec) Name() string { return predicates.UrlRawPathPrefixName }

func (*rawUrlPathPrefixSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	prefix, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	return &rawUrlPathPrefixPredicate{prefix: prefix}, nil
}

func (p *rawUrlPathPrefixPredicate) Match(r *http.Request) bool {
	return strings.HasPrefix(rawPath(r), p.prefix)
}

type rawUrlPathRegexpSpec struct{}

type rawUrlPathRegexpPredicate struct {
	re *regexp.Regexp
}

// NewUrlRawPathRegexp creates a predicate specification whose instances match
// requests where the raw (URL-encoded) path matches the given regular expression.
//
// It reads URL.RawPath when set, falling back to URL.Path. Use this when
// you need regexp matching on paths containing percent-encoded reserved
// characters such as %2F that Go's HTTP server decodes in URL.Path.
func NewUrlRawPathRegexp() routing.PredicateSpec { return &rawUrlPathRegexpSpec{} }

func (*rawUrlPathRegexpSpec) Name() string { return predicates.UrlRawPathRegexpName }

func (*rawUrlPathRegexpSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	pattern, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &rawUrlPathRegexpPredicate{re: re}, nil
}

func (p *rawUrlPathRegexpPredicate) Match(r *http.Request) bool {
	return p.re.MatchString(rawPath(r))
}
