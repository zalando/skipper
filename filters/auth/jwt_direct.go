package auth

import (
	"github.com/zalando/skipper/filters"
)

type jwtValidationKeysSpec struct{}

// NewJwtValidationKeys creates a filter spec for JWT validation using a direct JWKS URL.
//
// Unlike jwtValidation which discovers JWKS via .well-known/openid-configuration,
// this filter takes the JWKS URL directly. This is useful for services that publish
// JWKS keys at non-standard endpoints (e.g. Google Chat service accounts).
//
// The filter stores token claims into the state bag where they can be used by
// oidcClaimsQuery, forwardToken or forwardTokenField filters.
//
// Usage:
//
//	jwtValidationKeys("https://www.googleapis.com/service_accounts/v1/jwk/chat@system.gserviceaccount.com")
func NewJwtValidationKeys() filters.Spec {
	return &jwtValidationKeysSpec{}
}

func (s *jwtValidationKeysSpec) Name() string {
	return filters.JwtValidationKeysName
}

func (s *jwtValidationKeysSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	jwksURL := sargs[0]

	if err := registerKeyFunction(jwksURL); err != nil {
		return nil, err
	}

	return &jwtValidationFilter{
		jwksUri: jwksURL,
	}, nil
}
