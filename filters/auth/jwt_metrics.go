package auth

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/jwt"
)

type (
	jwtMetricsSpec struct{}

	jwtMetricsFilter struct {
		IgnoreStatusCodes []int            `json:"ignore_status_codes",omitempty`
		Claims            map[string][]any `json:"claims",omitempty`
	}
)

func NewJwtMetrics() filters.Spec {
	return &jwtMetricsSpec{}
}

func (s *jwtMetricsSpec) Name() string {
	return filters.JwtMetricsName
}

func (s *jwtMetricsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	f := &jwtMetricsFilter{
		IgnoreStatusCodes: []int{http.StatusUnauthorized, http.StatusForbidden},
	}

	if len(args) == 1 {
		if config, ok := args[0].(string); !ok {
			return nil, filters.ErrInvalidFilterParameters
		} else if err := yaml.Unmarshal([]byte(config), f); err != nil {
			return nil, fmt.Errorf(">>%w", err)
		}
	} else if len(args) > 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return f, nil
}

func (f *jwtMetricsFilter) Request(ctx filters.FilterContext) {}

func (f *jwtMetricsFilter) Response(ctx filters.FilterContext) {
	response := ctx.Response()

	if slices.Contains(f.IgnoreStatusCodes, response.StatusCode) {
		return
	}

	request := ctx.Request()

	metrics := ctx.Metrics()
	metricsPrefix := fmt.Sprintf("%s.%s.%d.", request.Method, escapeMetricKeySegment(request.Host), response.StatusCode)

	ahead := request.Header.Get("Authorization")
	if ahead == "" {
		metrics.IncCounter(metricsPrefix + "missing-token")
		return
	}

	tv := strings.TrimPrefix(ahead, "Bearer ")
	if tv == ahead {
		metrics.IncCounter(metricsPrefix + "invalid-token-type")
		return
	}

	token, err := jwt.Parse(tv)
	if err != nil {
		metrics.IncCounter(metricsPrefix + "invalid-token")
		return
	}

	hasAnyClaim := len(f.Claims) == 0
	for key, values := range f.Claims {
		if value, ok := token.Claims[key]; ok {
			for _, v := range values {
				if value == v {
					hasAnyClaim = true
					break
				}
			}
		}
	}

	if !hasAnyClaim {
		metrics.IncCounter(metricsPrefix + "missing-claim")
	}
}

var escapeMetricKeySegmentPattern = regexp.MustCompile("[^a-zA-Z0-9_]")

func escapeMetricKeySegment(s string) string {
	return escapeMetricKeySegmentPattern.ReplaceAllLiteralString(s, "_")
}
