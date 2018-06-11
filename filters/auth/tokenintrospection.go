package auth

import (
	"fmt"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	OAuthTokenintrospectionAnyClaimsName = "oauthTokenintrospectionAnyClaims"
	OAuthTokenintrospectionAllClaimsName = "oauthTokenintrospectionAllClaims"
	OAuthTokenintrospectionAnyKVName     = "oauthTokenintrospectionAnyKV"
	OAuthTokenintrospectionAllKVName     = "oauthTokenintrospectionAllKV"

	tokenintrospectionCacheKey   = "tokenintrospection"
	TokenIntrospectionConfigPath = "/.well-known/openid-configuration"
)

type (
	tokenIntrospectionSpec struct {
		typ              roleCheckType
		issuerURL        string
		introspectionURL string
		config           *OpenIDConfig
		authClient       *authClient
	}

	tokenIntrospectionInfo map[string]interface{}

	tokenintrospectFilter struct {
		typ        roleCheckType
		authClient *authClient
		claims     []string
		kv         kv
	}

	OpenIDConfig struct {
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
func NewOAuthTokenintrospectionAnyKV(cfg *OpenIDConfig) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAnyKV, cfg)
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
func NewOAuthTokenintrospectionAllKV(cfg *OpenIDConfig) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAllKV, cfg)
}

func NewOAuthTokenintrospectionAnyClaims(cfg *OpenIDConfig) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAnyClaims, cfg)
}

func NewOAuthTokenintrospectionAllClaims(cfg *OpenIDConfig) filters.Spec {
	return newOAuthTokenintrospectionFilter(checkOAuthTokenintrospectionAllClaims, cfg)
}

func newOAuthTokenintrospectionFilter(typ roleCheckType, cfg *OpenIDConfig) filters.Spec {
	return &tokenIntrospectionSpec{
		typ:              typ,
		issuerURL:        cfg.Issuer,
		introspectionURL: cfg.IntrospectionEndpoint,
		config:           cfg,
	}
}

func GetOpenIDConfig(issuerURL string) (*OpenIDConfig, error) {
	u, err := url.Parse(issuerURL + TokenIntrospectionConfigPath)
	if err != nil {
		return nil, err
	}

	var cfg OpenIDConfig
	err = jsonGet(u, "", &cfg)
	return &cfg, err
}

func (s *tokenIntrospectionSpec) Name() string {
	switch s.typ {
	case checkOAuthTokenintrospectionAnyClaims:
		return OAuthTokenintrospectionAnyClaimsName
	case checkOAuthTokenintrospectionAllClaims:
		return OAuthTokenintrospectionAllClaimsName
	case checkOAuthTokenintrospectionAnyKV:
		return OAuthTokenintrospectionAnyKVName
	case checkOAuthTokenintrospectionAllKV:
		return OAuthTokenintrospectionAllKVName
	}
	return AuthUnknown
}

func (s *tokenIntrospectionSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	ac, err := newAuthClient(s.introspectionURL)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &tokenintrospectFilter{
		typ:        s.typ,
		authClient: ac,
		kv:         make(map[string]string),
	}
	switch f.typ {
	case checkOAuthTokenintrospectionAllClaims:
		fallthrough
	case checkOAuthTokenintrospectionAnyClaims:
		f.claims = sargs[:]
		if s.config != nil && !all(f.claims, s.config.ClaimsSupported) {
			return nil, fmt.Errorf("%v: %s, supported Claims: %v", errUnsupportedClaimSpecified, strings.Join(f.claims, ","), s.config.ClaimsSupported)
		}

	// key value pairs
	case checkOAuthTokenintrospectionAllKV:
		fallthrough
	case checkOAuthTokenintrospectionAnyKV:
		for i := 0; i+1 < len(sargs); i += 2 {
			f.kv[sargs[i]] = sargs[i+1]
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
		return fmt.Sprintf("%s(%s)", OAuthTokenintrospectionAnyClaimsName, strings.Join(f.claims, ","))
	case checkOAuthTokenintrospectionAllClaims:
		return fmt.Sprintf("%s(%s)", OAuthTokenintrospectionAllClaimsName, strings.Join(f.claims, ","))
	case checkOAuthTokenintrospectionAnyKV:
		return fmt.Sprintf("%s(%s)", OAuthTokenintrospectionAnyKVName, f.kv)
	case checkOAuthTokenintrospectionAllKV:
		return fmt.Sprintf("%s(%s)", OAuthTokenintrospectionAllKVName, f.kv)
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
		v2, ok := info[k].(string)
		if !ok || v != v2 {
			return false
		}
	}
	return true
}

func (f *tokenintrospectFilter) validateAnyKV(info tokenIntrospectionInfo) bool {
	for k, v := range f.kv {
		v2, ok := info[k].(string)
		if ok && v == v2 {
			return true
		}
	}
	return false
}

func (f *tokenintrospectFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info tokenIntrospectionInfo
	infoTemp, ok := ctx.StateBag()[tokenintrospectionCacheKey]
	if !ok {
		token, err := getToken(r)
		if err != nil {
			unauthorized(ctx, "", missingToken, f.authClient.url.Hostname())
			return
		}

		if token == "" {
			unauthorized(ctx, "", missingToken, f.authClient.url.Hostname())
			return
		}

		info, err = f.authClient.getTokenintrospect(token)
		if err != nil {
			reason := authServiceAccess
			if err == errInvalidToken {
				reason = invalidToken
			}
			unauthorized(ctx, "", reason, f.authClient.url.Hostname())
			return
		}
	} else {
		info = infoTemp.(tokenIntrospectionInfo)
	}

	sub, err := info.Sub()
	if err != nil {
		unauthorized(ctx, sub, invalidSub, f.authClient.url.Hostname())
		return
	}

	if !info.Active() {
		unauthorized(ctx, sub, inactiveToken, f.authClient.url.Hostname())
		return
	}

	var allowed bool
	switch f.typ {
	case checkOAuthTokenintrospectionAnyClaims:
		allowed = f.validateAnyClaims(info)
	case checkOAuthTokenintrospectionAnyKV:
		allowed = f.validateAnyKV(info)
	case checkOAuthTokenintrospectionAllClaims:
		allowed = f.validateAllClaims(info)
	case checkOAuthTokenintrospectionAllKV:
		allowed = f.validateAllKV(info)
	default:
		log.Errorf("Wrong tokenintrospectionFilter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, sub, invalidClaim, f.authClient.url.Hostname())
		return
	}
	authorized(ctx, sub)
	ctx.StateBag()[tokenintrospectionCacheKey] = info
}

func (f *tokenintrospectFilter) Response(filters.FilterContext) {}
