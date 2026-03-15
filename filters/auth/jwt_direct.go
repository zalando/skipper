package auth

import (
	"fmt"

	"github.com/zalando/skipper/filters"
)

type (
	jwtDirectSpec struct{}

	jwtDirectFilter struct {
		jwksURL string
		claims  map[string]string
	}
)

// NewJwtDirect creates a filter spec for JWT validation using a direct JWKS URL
// and optional claim key-value validation.
//
// Usage:
//
//	jwtDirect("https://example.com/jwks", "iss", "expected-issuer", "aud", "expected-audience")
//
// The first argument is the JWKS URL. Remaining arguments are key-value pairs
// specifying claims that must match exactly. All specified claims must be present
// and match for the token to be accepted.
func NewJwtDirect() filters.Spec {
	return &jwtDirectSpec{}
}

func (s *jwtDirectSpec) Name() string {
	return filters.JwtDirectName
}

func (s *jwtDirectSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	jwksURL := sargs[0]

	// Remaining args must be key-value pairs
	kvArgs := sargs[1:]
	if len(kvArgs)%2 != 0 {
		return nil, fmt.Errorf("%w: claim key-value pairs must be even", filters.ErrInvalidFilterParameters)
	}

	claims := make(map[string]string, len(kvArgs)/2)
	for i := 0; i < len(kvArgs); i += 2 {
		claims[kvArgs[i]] = kvArgs[i+1]
	}

	if err := registerKeyFunction(jwksURL); err != nil {
		return nil, err
	}

	return &jwtDirectFilter{
		jwksURL: jwksURL,
		claims:  claims,
	}, nil
}

func (f *jwtDirectFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info tokenContainer
	infoTemp, ok := ctx.StateBag()[oidcClaimsCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingToken, "", "")
			return
		}

		parsedClaims, err := parseToken(token, f.jwksURL)
		if err != nil {
			ctx.Logger().Errorf("Error while parsing jwt token: %v.", err)
			unauthorized(ctx, "", invalidToken, "", "")
			return
		}

		info.Claims = parsedClaims
	} else {
		info = infoTemp.(tokenContainer)
	}

	// Validate required claims
	for key, expected := range f.claims {
		actual, ok := claimAsString(info.Claims[key])
		if !ok || actual != expected {
			unauthorized(ctx, "", invalidClaim, "", fmt.Sprintf("claim %q: got %q, want %q", key, actual, expected))
			return
		}
	}

	sub, _ := info.Claims["sub"].(string)
	authorized(ctx, sub)

	ctx.StateBag()[oidcClaimsCacheKey] = info
}

func (f *jwtDirectFilter) Response(filters.FilterContext) {}

func claimAsString(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case float64:
		return fmt.Sprintf("%v", val), true
	default:
		return "", false
	}
}
