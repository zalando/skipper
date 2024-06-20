package auth

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/jwt"
)

type (
	jwtMetricsSpec struct{}

	jwtMetricsFilter struct {
		Issuers           []string `json:"issuers,omitempty"`
		OptOutAnnotations []string `json:"optOutAnnotations,omitempty"`
		OptOutStateBag    []string `json:"optOutStateBag,omitempty"`
	}
)

func NewJwtMetrics() filters.Spec {
	return &jwtMetricsSpec{}
}

func (s *jwtMetricsSpec) Name() string {
	return filters.JwtMetricsName
}

func (s *jwtMetricsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f := &jwtMetricsFilter{}

	if len(args) == 1 {
		if config, ok := args[0].(string); !ok {
			return nil, fmt.Errorf("requires single string argument")
		} else if err := yaml.Unmarshal([]byte(config), f); err != nil {
			return nil, fmt.Errorf("failed to parse configuration")
		}
	} else if len(args) > 1 {
		return nil, fmt.Errorf("requires single string argument")
	}

	return f, nil
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

	if len(f.Issuers) > 0 {
		token, err := jwt.Parse(tv)
		if err != nil {
			count("invalid-token")
			return
		}

		// https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1
		if issuer, ok := token.Claims["iss"].(string); !ok {
			count("missing-issuer")
		} else if !slices.Contains(f.Issuers, issuer) {
			count("invalid-issuer")
		}
	}
}

var escapeMetricKeySegmentPattern = regexp.MustCompile("[^a-zA-Z0-9_]")

func escapeMetricKeySegment(s string) string {
	return escapeMetricKeySegmentPattern.ReplaceAllLiteralString(s, "_")
}
