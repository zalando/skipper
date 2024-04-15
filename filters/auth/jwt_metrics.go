package auth

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/jwt"
)

type (
	jwtMetricsSpec struct{}

	jwtMetricsFilter struct {
		claims [][2]string
	}
)

func NewJwtMetrics() filters.Spec {
	return &jwtMetricsSpec{}
}

func (s *jwtMetricsSpec) Name() string {
	return filters.JwtMetricsName
}

func (s *jwtMetricsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args)%2 != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	f := &jwtMetricsFilter{
		claims: make([][2]string, 0, len(args)/2),
	}
	for i := 0; i < len(args); i += 2 {
		key, keyOk := args[i].(string)
		value, valueOk := args[i+1].(string)
		if !keyOk || !valueOk {
			return nil, filters.ErrInvalidFilterParameters
		}
		f.claims = append(f.claims, [2]string{key, value})
	}
	return f, nil
}

func (f *jwtMetricsFilter) Request(ctx filters.FilterContext) {}

func (f *jwtMetricsFilter) Response(ctx filters.FilterContext) {
	response := ctx.Response()

	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return // ignore request that failed authentication
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

	hasAnyClaim := false
	for _, claim := range f.claims {
		key, expected := claim[0], claim[1]
		if value, ok := token.Claims[key]; ok && value == expected {
			hasAnyClaim = true
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
