package auth

import (
	"fmt"

	"github.com/zalando/skipper/filters"
	"golang.org/x/net/http/httpguts"
)

const (
	// Deprecated, use filters.ForwardTokenFieldName instead
	ForwardTokenFieldName = filters.ForwardTokenFieldName
)

type (
	forwardTokenFieldSpec   struct{}
	forwardTokenFieldFilter struct {
		HeaderName string
		Field      string
	}
)

// NewForwardTokenField creates a filter to forward fields from token info or
// token introspection or oidc user info as headers to the backend server.
func NewForwardTokenField() filters.Spec {
	return &forwardTokenFieldSpec{}
}

func (s *forwardTokenFieldSpec) Name() string {
	return filters.ForwardTokenFieldName
}

func (*forwardTokenFieldSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	headerName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	valid := httpguts.ValidHeaderFieldName(headerName)
	if !valid {
		return nil, fmt.Errorf("header name %s is invalid", headerName)
	}

	field, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &forwardTokenFieldFilter{HeaderName: headerName, Field: field}, nil
}

func (f *forwardTokenFieldFilter) Request(ctx filters.FilterContext) {
	payload := getPayload(ctx, tokeninfoCacheKey)
	if payload == nil {
		payload = getPayload(ctx, tokenintrospectionCacheKey)
	}
	if payload == nil {
		payload = getPayload(ctx, oidcClaimsCacheKey)
	}
	if payload == nil {
		return
	}

	err := setHeaders(map[string]string{
		f.HeaderName: f.Field,
	}, ctx, payload)

	if err != nil {
		ctx.Logger().Errorf("%v", err)
		return
	}
}

func (*forwardTokenFieldFilter) Response(filters.FilterContext) {}

func getPayload(ctx filters.FilterContext, cacheKey string) any {
	cachedValue, ok := ctx.StateBag()[cacheKey]
	if !ok {
		return nil
	}

	return cachedValue
}
