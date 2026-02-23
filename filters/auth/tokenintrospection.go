package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.OAuthTokenintrospectionAnyClaimsName instead
	OAuthTokenintrospectionAnyClaimsName = filters.OAuthTokenintrospectionAnyClaimsName
	// Deprecated, use filters.OAuthTokenintrospectionAllClaimsName instead
	OAuthTokenintrospectionAllClaimsName = filters.OAuthTokenintrospectionAllClaimsName
	// Deprecated, use filters.OAuthTokenintrospectionAnyKVName instead
	OAuthTokenintrospectionAnyKVName = filters.OAuthTokenintrospectionAnyKVName
	// Deprecated, use filters.OAuthTokenintrospectionAllKVName instead
	OAuthTokenintrospectionAllKVName = filters.OAuthTokenintrospectionAllKVName
	// Deprecated, use filters.SecureOAuthTokenintrospectionAnyClaimsName instead
	SecureOAuthTokenintrospectionAnyClaimsName = filters.SecureOAuthTokenintrospectionAnyClaimsName
	// Deprecated, use filters.SecureOAuthTokenintrospectionAllClaimsName instead
	SecureOAuthTokenintrospectionAllClaimsName = filters.SecureOAuthTokenintrospectionAllClaimsName
	// Deprecated, use filters.SecureOAuthTokenintrospectionAnyKVName instead
	SecureOAuthTokenintrospectionAnyKVName = filters.SecureOAuthTokenintrospectionAnyKVName
	// Deprecated, use filters.SecureOAuthTokenintrospectionAllKVName instead
	SecureOAuthTokenintrospectionAllKVName = filters.SecureOAuthTokenintrospectionAllKVName

	tokenintrospectionCacheKey   = "tokenintrospection"
	TokenIntrospectionConfigPath = "/.well-known/openid-configuration"
)

type TokenintrospectionOptions struct {
	Timeout      time.Duration
	Tracer       opentracing.Tracer
	MaxIdleConns int

	// OpenTracingClientTraceByTag instead of events use span Tags
	// to measure client connection pool actions
	OpenTracingClientTraceByTag bool
}

type (
	tokenIntrospectionSpec struct {
		typ     roleCheckType
		options TokenintrospectionOptions
		secure  bool
	}

	tokenIntrospectionInfo map[string]interface{}

	tokenintrospectFilter struct {
		typ        roleCheckType
		authClient *authClient
		claims     []string
		kv         kv
	}

	openIDConfig struct {
		Issuer                            string   `json:"issuer"`
		AuthorizationEndpoint             string   `json:"authorization_endpoint"`
		TokenEndpoint                     string   `json:"token_endpoint"`
		UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
		RevocationEndpoint                string   `json:"revocation_endpoint"`
		JwksURI                           string   `json:"jwks_uri"`
		RegistrationEndpoint              string   `json:"registration_endpoint"`
		IntrospectionEndpoint             string   `json:"introspection_endpoint"`
		ResponseTypesSupported            []string `json:"response_types_supported"`
		SubjectTypesSupported             []string `json:"subject_types_supported"`
		IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
		TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
		ClaimsSupported                   []string `json:"claims_supported"`
		ScopesSupported                   []string `json:"scopes_supported"`
		CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	}
)

var issuerAuthClient map[string]*authClient = make(map[string]*authClient)

// Active returns token introspection response, which is true if token
// is not revoked and in the time frame of
// validity. https://tools.ietf.org/html/rfc7662#section-2.2
func (tii tokenIntrospectionInfo) Active() bool {
	return tii.getBoolValue("active")
}

func (tii tokenIntrospectionInfo) Sub() (string, error) {
	return tii.getStringValue("sub")
}

func (tii tokenIntrospectionInfo) getBoolValue(k string) bool {
	if active, ok := tii[k].(bool); ok {
		return active
	}
	return false
}

func (tii tokenIntrospectionInfo) getStringValue(k string) (string, error) {
	s, ok := tii[k].(string)
	if !ok {
		return "", errInvalidTokenintrospectionData
	}
	return s, nil
}

// NewOAuthTokenintrospectionAnyKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one key value pair provided.
//
// This is implementing RFC 7662 compliant implementation. It uses
// POST requests to call introspection_endpoint to get the information
// of the token validity.
//
// It uses /.well-known/openid-configuration path to the passed
// oauthIssuerURL to find introspection_endpoint as defined in draft
// https://tools.ietf.org/html/draft-ietf-oauth-discovery-06, if
// oauthIntrospectionURL is a non empty string, it will set
// IntrospectionEndpoint to the given oauthIntrospectionURL.
func NewOAuthTokenintrospectionAnyKV(timeout time.Duration) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAnyKV, timeout)
}

// NewOAuthTokenintrospectionAllKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one key value pair provided.
//
// This is implementing RFC 7662 compliant implementation. It uses
// POST requests to call introspection_endpoint to get the information
// of the token validity.
//
// It uses /.well-known/openid-configuration path to the passed
// oauthIssuerURL to find introspection_endpoint as defined in draft
// https://tools.ietf.org/html/draft-ietf-oauth-discovery-06, if
// oauthIntrospectionURL is a non empty string, it will set
// IntrospectionEndpoint to the given oauthIntrospectionURL.
func NewOAuthTokenintrospectionAllKV(timeout time.Duration) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAllKV, timeout)
}

func NewOAuthTokenintrospectionAnyClaims(timeout time.Duration) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAnyClaims, timeout)
}

func NewOAuthTokenintrospectionAllClaims(timeout time.Duration) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAllClaims, timeout)
}

func NewSecureOAuthTokenintrospectionAnyKV(timeout time.Duration) filters.Spec {
	return newSecureOAuthTokenintrospectionFilter(checkSecureOAuthTokenintrospectionAnyKV, timeout)
}
func NewSecureOAuthTokenintrospectionAllKV(timeout time.Duration) filters.Spec {
	return newSecureOAuthTokenintrospectionFilter(checkSecureOAuthTokenintrospectionAllKV, timeout)
}

func NewSecureOAuthTokenintrospectionAnyClaims(timeout time.Duration) filters.Spec {
	return newSecureOAuthTokenintrospectionFilter(checkSecureOAuthTokenintrospectionAnyClaims, timeout)
}

func NewSecureOAuthTokenintrospectionAllClaims(timeout time.Duration) filters.Spec {
	return newSecureOAuthTokenintrospectionFilter(checkSecureOAuthTokenintrospectionAllClaims, timeout)
}

// TokenintrospectionWithOptions create a new auth filter specification
// for validating authorization requests with additional options to the
// mandatory timeout parameter.
//
// Use one of the base initializer functions as the first argument:
// NewOAuthTokenintrospectionAnyKV, NewOAuthTokenintrospectionAllKV,
// NewOAuthTokenintrospectionAnyClaims, NewOAuthTokenintrospectionAllClaims,
// NewSecureOAuthTokenintrospectionAnyKV, NewSecureOAuthTokenintrospectionAllKV,
// NewSecureOAuthTokenintrospectionAnyClaims, NewSecureOAuthTokenintrospectionAllClaims,
// pass opentracing.Tracer and other options in TokenintrospectionOptions.
func TokenintrospectionWithOptions(
	create func(time.Duration) filters.Spec,
	o TokenintrospectionOptions,
) filters.Spec {
	s := create(o.Timeout)
	ts, ok := s.(*tokenIntrospectionSpec)
	if !ok {
		return s
	}

	ts.options = o
	return ts
}

func newOAuthTokenintrospectionFilter(typ roleCheckType, timeout time.Duration) filters.Spec {
	return &tokenIntrospectionSpec{
		typ: typ,
		options: TokenintrospectionOptions{
			Timeout: timeout,
			Tracer:  opentracing.NoopTracer{},
		},
		secure: false,
	}
}

func newSecureOAuthTokenintrospectionFilter(typ roleCheckType, timeout time.Duration) filters.Spec {
	return &tokenIntrospectionSpec{
		typ: typ,
		options: TokenintrospectionOptions{
			Timeout: timeout,
			Tracer:  opentracing.NoopTracer{},
		},
		secure: true,
	}
}

func getOpenIDConfig(issuerURL string) (*openIDConfig, error) {
	u, err := url.Parse(issuerURL + TokenIntrospectionConfigPath)
	if err != nil {
		return nil, err
	}

	rsp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		return nil, errInvalidToken
	}
	d := json.NewDecoder(rsp.Body)
	var cfg openIDConfig
	err = d.Decode(&cfg)
	return &cfg, err
}

func (s *tokenIntrospectionSpec) Name() string {
	switch s.typ {
	case checkOAuthTokenintrospectionAnyClaims:
		return filters.OAuthTokenintrospectionAnyClaimsName
	case checkOAuthTokenintrospectionAllClaims:
		return filters.OAuthTokenintrospectionAllClaimsName
	case checkOAuthTokenintrospectionAnyKV:
		return filters.OAuthTokenintrospectionAnyKVName
	case checkOAuthTokenintrospectionAllKV:
		return filters.OAuthTokenintrospectionAllKVName
	case checkSecureOAuthTokenintrospectionAnyClaims:
		return filters.SecureOAuthTokenintrospectionAnyClaimsName
	case checkSecureOAuthTokenintrospectionAllClaims:
		return filters.SecureOAuthTokenintrospectionAllClaimsName
	case checkSecureOAuthTokenintrospectionAnyKV:
		return filters.SecureOAuthTokenintrospectionAnyKVName
	case checkSecureOAuthTokenintrospectionAllKV:
		return filters.SecureOAuthTokenintrospectionAllKVName
	}
	return AuthUnknown
}

func (s *tokenIntrospectionSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if s.secure && len(sargs) < 4 || !s.secure && len(sargs) < 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	issuerURL := sargs[0]

	var clientId, clientSecret string

	if s.secure {
		clientId = sargs[1]
		clientSecret = sargs[2]
		sargs = sargs[3:]
		if clientId == "" {
			clientId, _ = os.LookupEnv("OAUTH_CLIENT_ID")
		}

		if clientSecret == "" {
			clientSecret, _ = os.LookupEnv("OAUTH_CLIENT_SECRET")
		}
	} else {
		sargs = sargs[1:]
	}

	cfg, err := getOpenIDConfig(issuerURL)
	if err != nil {
		return nil, err
	}

	var ac *authClient
	var ok bool
	if ac, ok = issuerAuthClient[issuerURL]; !ok {
		ac, err = newAuthClient(cfg.IntrospectionEndpoint, tokenIntrospectionSpanName, s.options.Timeout, s.options.MaxIdleConns, s.options.Tracer, s.options.OpenTracingClientTraceByTag)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}
		issuerAuthClient[issuerURL] = ac
	}

	if s.secure && clientId != "" && clientSecret != "" {
		ac.url.User = url.UserPassword(clientId, clientSecret)
	} else {
		ac.url.User = nil
	}

	f := &tokenintrospectFilter{
		typ:        s.typ,
		authClient: ac,
		kv:         make(map[string][]string),
	}
	switch f.typ {
	case checkOAuthTokenintrospectionAllClaims:
		fallthrough
	case checkSecureOAuthTokenintrospectionAllClaims:
		fallthrough
	case checkSecureOAuthTokenintrospectionAnyClaims:
		fallthrough
	case checkOAuthTokenintrospectionAnyClaims:
		f.claims = sargs
		if !all(f.claims, cfg.ClaimsSupported) {
			return nil, fmt.Errorf("%w: %s, supported Claims: %v", errUnsupportedClaimSpecified, strings.Join(f.claims, ","), cfg.ClaimsSupported)
		}

	// key value pairs
	case checkOAuthTokenintrospectionAllKV:
		fallthrough
	case checkSecureOAuthTokenintrospectionAllKV:
		fallthrough
	case checkSecureOAuthTokenintrospectionAnyKV:
		fallthrough
	case checkOAuthTokenintrospectionAnyKV:
		for i := 0; i+1 < len(sargs); i += 2 {
			f.kv[sargs[i]] = append(f.kv[sargs[i]], sargs[i+1])
		}
		if len(sargs) == 0 || len(sargs)%2 != 0 {
			return nil, filters.ErrInvalidFilterParameters
		}
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	return f, nil
}

// String prints nicely the tokenintrospectFilter configuration based on the
// configuration and check used.
func (f *tokenintrospectFilter) String() string {
	switch f.typ {
	case checkOAuthTokenintrospectionAnyClaims:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokenintrospectionAnyClaimsName, strings.Join(f.claims, ","))
	case checkOAuthTokenintrospectionAllClaims:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokenintrospectionAllClaimsName, strings.Join(f.claims, ","))
	case checkOAuthTokenintrospectionAnyKV:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokenintrospectionAnyKVName, f.kv)
	case checkOAuthTokenintrospectionAllKV:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokenintrospectionAllKVName, f.kv)
	case checkSecureOAuthTokenintrospectionAnyClaims:
		return fmt.Sprintf("%s(%s)", filters.SecureOAuthTokenintrospectionAnyClaimsName, strings.Join(f.claims, ","))
	case checkSecureOAuthTokenintrospectionAllClaims:
		return fmt.Sprintf("%s(%s)", filters.SecureOAuthTokenintrospectionAllClaimsName, strings.Join(f.claims, ","))
	case checkSecureOAuthTokenintrospectionAnyKV:
		return fmt.Sprintf("%s(%s)", filters.SecureOAuthTokenintrospectionAnyKVName, f.kv)
	case checkSecureOAuthTokenintrospectionAllKV:
		return fmt.Sprintf("%s(%s)", filters.SecureOAuthTokenintrospectionAllKVName, f.kv)
	}
	return AuthUnknown
}

func (f *tokenintrospectFilter) validateAnyClaims(info tokenIntrospectionInfo) bool {
	for _, wantedClaim := range f.claims {
		if claims, ok := info["claims"].(map[string]interface{}); ok {
			if _, ok2 := claims[wantedClaim]; ok2 {
				return true
			}
		}
	}
	return false
}

func (f *tokenintrospectFilter) validateAllClaims(info tokenIntrospectionInfo) bool {
	for _, v := range f.claims {
		if claims, ok := info["claims"].(map[string]interface{}); !ok {
			return false
		} else {
			if _, ok := claims[v]; !ok {
				return false
			}
		}
	}
	return true
}

func (f *tokenintrospectFilter) validateAllKV(info tokenIntrospectionInfo) bool {
	for k, v := range f.kv {
		for _, res := range v {
			v2, ok := info[k].(string)
			if !ok || res != v2 {
				return false
			}
		}
	}
	return true
}

func (f *tokenintrospectFilter) validateAnyKV(info tokenIntrospectionInfo) bool {
	for k, v := range f.kv {
		for _, res := range v {
			v2, ok := info[k].(string)
			if ok && res == v2 {
				return true
			}
		}
	}
	return false
}

func (f *tokenintrospectFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info tokenIntrospectionInfo
	infoTemp, ok := ctx.StateBag()[tokenintrospectionCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingToken, f.authClient.url.Hostname(), "")
			return
		}

		var err error
		info, err = f.authClient.getTokenintrospect(token, ctx)
		if err != nil {
			reason := authServiceAccess
			if err == errInvalidToken {
				reason = invalidToken
			} else {
				ctx.Logger().Errorf("Error while calling token introspection: %v", err)
			}

			unauthorized(ctx, "", reason, f.authClient.url.Hostname(), "")
			return
		}
	} else {
		info = infoTemp.(tokenIntrospectionInfo)
	}

	sub, err := info.Sub()
	if err != nil {
		if err != errInvalidTokenintrospectionData {
			ctx.Logger().Errorf("Error while reading token: %v", err)
		}

		unauthorized(ctx, sub, invalidSub, f.authClient.url.Hostname(), "")
		return
	}

	if !info.Active() {
		unauthorized(ctx, sub, inactiveToken, f.authClient.url.Hostname(), "")
		return
	}

	var allowed bool
	switch f.typ {
	case checkOAuthTokenintrospectionAnyClaims, checkSecureOAuthTokenintrospectionAnyClaims:
		allowed = f.validateAnyClaims(info)
	case checkOAuthTokenintrospectionAnyKV, checkSecureOAuthTokenintrospectionAnyKV:
		allowed = f.validateAnyKV(info)
	case checkOAuthTokenintrospectionAllClaims, checkSecureOAuthTokenintrospectionAllClaims:
		allowed = f.validateAllClaims(info)
	case checkOAuthTokenintrospectionAllKV, checkSecureOAuthTokenintrospectionAllKV:
		allowed = f.validateAllKV(info)
	default:
		ctx.Logger().Errorf("Wrong tokenintrospectionFilter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, sub, invalidClaim, f.authClient.url.Hostname(), "")
		return
	}

	authorized(ctx, sub)
	ctx.StateBag()[tokenintrospectionCacheKey] = info
}

func (f *tokenintrospectFilter) Response(filters.FilterContext) {}
