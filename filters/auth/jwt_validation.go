package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc"
	jwt "github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.JwtValidationName instead
	JwtValidationName = filters.JwtValidationName
)

type (
	jwtValidationSpec struct {
		options TokenintrospectionOptions
	}

	jwtValidationFilter struct {
		jwksUri string
	}
)

var refreshInterval = time.Hour
var refreshRateLimit = time.Minute * 5
var refreshTimeout = time.Second * 10
var refreshUnknownKID = true

// the map of jwks keyfunctions stored per jwksUri
var (
	jwksMu  sync.RWMutex
	jwksMap map[string]*keyfunc.JWKS = make(map[string]*keyfunc.JWKS)
)

func NewJwtValidationWithOptions(o TokenintrospectionOptions) filters.Spec {
	return &jwtValidationSpec{
		options: o,
	}
}

func (s *jwtValidationSpec) Name() string {
	return filters.JwtValidationName
}

func (s *jwtValidationSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	issuerURL := sargs[0]

	cfg, err := getOpenIDConfig(issuerURL)
	if err != nil {
		return nil, err
	}

	err = registerKeyFunction(cfg.JwksURI)
	if err != nil {
		return nil, err
	}

	f := &jwtValidationFilter{
		jwksUri: cfg.JwksURI,
	}

	return f, nil
}

func hasKeyFunction(url string) bool {
	jwksMu.RLock()
	defer jwksMu.RUnlock()

	_, ok := jwksMap[url]
	return ok
}

func putKeyFunction(url string, jwks *keyfunc.JWKS) {
	jwksMu.Lock()
	defer jwksMu.Unlock()

	jwksMap[url] = jwks
}

func registerKeyFunction(url string) (err error) {
	if hasKeyFunction(url) {
		return nil
	}

	options := keyfunc.Options{
		RefreshErrorHandler: func(err error) {
			log.Errorf("There was an error on key refresh for the given URL %s\nError:%s\n", url, err.Error())
		},
		RefreshInterval:   refreshInterval,
		RefreshRateLimit:  refreshRateLimit,
		RefreshTimeout:    refreshTimeout,
		RefreshUnknownKID: refreshUnknownKID,
	}

	jwks, err := keyfunc.Get(url, options)
	if err != nil {
		return fmt.Errorf("failed to get the JWKS from the given URL %s Error:%w", url, err)
	}

	putKeyFunction(url, jwks)
	return nil
}

func getKeyFunction(url string) (jwks *keyfunc.JWKS) {
	jwksMu.RLock()
	defer jwksMu.RUnlock()

	return jwksMap[url]
}

func (f *jwtValidationFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info tokenContainer
	infoTemp, ok := ctx.StateBag()[oidcClaimsCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingToken, "", "")
			return
		}

		claims, err := parseToken(token, f.jwksUri)
		if err != nil {
			ctx.Logger().Errorf("Error while parsing jwt token : %v.", err)
			unauthorized(ctx, "", invalidToken, "", "")
			return
		}

		info.Claims = claims
	} else {
		info = infoTemp.(tokenContainer)
	}

	sub, ok := info.Claims["sub"].(string)
	if !ok {
		unauthorized(ctx, sub, invalidSub, "", "")
		return
	}

	authorized(ctx, sub)

	ctx.StateBag()[oidcClaimsCacheKey] = info
}

func (f *jwtValidationFilter) Response(filters.FilterContext) {}

func parseToken(token string, jwksUri string) (map[string]any, error) {
	jwks := getKeyFunction(jwksUri)

	var claims jwt.MapClaims
	parsedToken, err := jwt.ParseWithClaims(token, &claims, jwks.Keyfunc)
	if err != nil {
		return nil, fmt.Errorf("error while parsing jwt token : %w", err)
	} else if !parsedToken.Valid {
		return nil, fmt.Errorf("invalid token")
	} else {
		return claims, nil
	}
}
