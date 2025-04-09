package auth

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/jwt"
)

type (
	jwtMetricsSpec struct {
		yamlConfigParser[jwtMetricsFilter]
	}

	// jwtMetricsFilter implements [yamlConfig],
	// make sure it is not modified after initialization.
	jwtMetricsFilter struct {
		// Issuers is *DEPRECATED* and will be removed in the future. Use the Claims field instead.
		Issuers           []string         `json:"issuers,omitempty"`
		OptOutAnnotations []string         `json:"optOutAnnotations,omitempty"`
		OptOutStateBag    []string         `json:"optOutStateBag,omitempty"`
		OptOutHosts       []string         `json:"optOutHosts,omitempty"`
		Claims            []map[string]any `json:"claims,omitempty"`

		optOutHostsCompiled []*regexp.Regexp
	}
)

func NewJwtMetrics() filters.Spec {
	return &jwtMetricsSpec{
		newYamlConfigParser[jwtMetricsFilter](64),
	}
}

func (s *jwtMetricsSpec) Name() string {
	return filters.JwtMetricsName
}

func (s *jwtMetricsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return &jwtMetricsFilter{}, nil
	}
	return s.parseSingleArg(args)
}

func (f *jwtMetricsFilter) initialize() error {
	for _, host := range f.OptOutHosts {
		if r, err := regexp.Compile(host); err != nil {
			return fmt.Errorf("failed to compile opt-out host pattern: %q", host)
		} else {
			f.optOutHostsCompiled = append(f.optOutHostsCompiled, r)
		}
	}
	return nil
}

func (f *jwtMetricsFilter) Request(ctx filters.FilterContext) {}

func (f *jwtMetricsFilter) Response(ctx filters.FilterContext) {
	if len(f.OptOutAnnotations) > 0 {
		annotations := annotate.GetAnnotations(ctx)
		for _, annotation := range f.OptOutAnnotations {
			if _, ok := annotations[annotation]; ok {
				return // opt-out
			}
		}
	}

	if len(f.OptOutStateBag) > 0 {
		sb := ctx.StateBag()
		for _, key := range f.OptOutStateBag {
			if _, ok := sb[key]; ok {
				return // opt-out
			}
		}
	}

	if len(f.optOutHostsCompiled) > 0 {
		host := ctx.Request().Host
		for _, r := range f.optOutHostsCompiled {
			if r.MatchString(host) {
				return // opt-out
			}
		}
	}

	response := ctx.Response()

	if response.StatusCode >= 400 && response.StatusCode < 500 {
		return // ignore invalid requests
	}

	request := ctx.Request()

	count := func(metric string) {
		prefix := fmt.Sprintf("%s.%s.%d.", request.Method, escapeMetricKeySegment(request.Host), response.StatusCode)

		ctx.Metrics().IncCounter(prefix + metric)

		if span := opentracing.SpanFromContext(ctx.Request().Context()); span != nil {
			span.SetTag("jwt", metric)
		}
	}

	ahead := request.Header.Get("Authorization")
	if ahead == "" {
		count("missing-token")
		return
	}

	tv := strings.TrimPrefix(ahead, "Bearer ")
	if tv == ahead {
		count("invalid-token-type")
		return
	}

	if len(f.Issuers) > 0 || len(f.Claims) > 0 {
		token, err := jwt.Parse(tv)
		if err != nil {
			count("invalid-token")
			return
		}

		if len(f.Issuers) > 0 {
			// https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1
			if issuer, ok := token.Claims["iss"].(string); !ok {
				count("missing-issuer")
			} else if !slices.Contains(f.Issuers, issuer) {
				count("invalid-issuer")
			}
		}

		if len(f.Claims) > 0 {
			found := false
			for _, claim := range f.Claims {
				if containsAll(token.Claims, claim) {
					found = true
					break
				}
			}
			if !found {
				count("invalid-claims")
			}
		}
	}
}

var escapeMetricKeySegmentPattern = regexp.MustCompile("[^a-zA-Z0-9_]")

func escapeMetricKeySegment(s string) string {
	return escapeMetricKeySegmentPattern.ReplaceAllLiteralString(s, "_")
}

// containsAll returns true if all key-values of b are present in a.
func containsAll(a, b map[string]any) bool {
	for kb, vb := range b {
		if va, ok := a[kb]; !ok || va != vb {
			return false
		}
	}
	return true
}
