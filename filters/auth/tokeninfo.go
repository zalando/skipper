package auth

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	OAuthTokeninfoAnyScopeName = "oauthTokeninfoAnyScope"
	OAuthTokeninfoAllScopeName = "oauthTokeninfoAllScope"
	OAuthTokeninfoAnyKVName    = "oauthTokeninfoAnyKV"
	OAuthTokeninfoAllKVName    = "oauthTokeninfoAllKV"
	tokeninfoCacheKey          = "tokeninfo"
)

type TokeninfoOptions struct {
	URL          string
	Timeout      time.Duration
	MaxIdleConns int
}

type (
	tokeninfoSpec struct {
		typ     roleCheckType
		options TokeninfoOptions
	}

	tokeninfoFilter struct {
		typ        roleCheckType
		authClient *authClient
		scopes     []string
		kv         kv
	}
)

var tokeninfoAuthClient map[string]*authClient = make(map[string]*authClient)

// NewOAuthTokeninfoAllScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all scopes.
func NewOAuthTokeninfoAllScope(oauthTokeninfoURL string, oauthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{
		typ: checkOAuthTokeninfoAllScopes,
		options: TokeninfoOptions{
			URL:     oauthTokeninfoURL,
			Timeout: oauthTokeninfoTimeout,
		},
	}
}

// NewOAuthTokeninfoAnyScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one scope.
func NewOAuthTokeninfoAnyScope(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{
		typ: checkOAuthTokeninfoAnyScopes,
		options: TokeninfoOptions{
			URL:     OAuthTokeninfoURL,
			Timeout: OAuthTokeninfoTimeout,
		},
	}
}

// NewOAuthTokeninfoAllKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all key value pairs provided.
func NewOAuthTokeninfoAllKV(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{
		typ: checkOAuthTokeninfoAllKV,
		options: TokeninfoOptions{
			URL:     OAuthTokeninfoURL,
			Timeout: OAuthTokeninfoTimeout,
		},
	}
}

// NewOAuthTokeninfoAnyKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one key value pair provided.
func NewOAuthTokeninfoAnyKV(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{
		typ: checkOAuthTokeninfoAnyKV,
		options: TokeninfoOptions{
			URL:     OAuthTokeninfoURL,
			Timeout: OAuthTokeninfoTimeout,
		},
	}
}

// TokeninfoWithOptions creates a new auth filter specification
// for token validation with additional settings to the mandatory
// tokeninfo URL and timeout.
//
// Use one of the base initializer functions as the first argument:
// NewOAuthTokeninfoAllScope, NewOAuthTokeninfoAnyScope,
// NewOAuthTokeninfoAllKV or NewOAuthTokeninfoAnyKV.
//
func TokeninfoWithOptions(create func(string, time.Duration) filters.Spec, o TokeninfoOptions) filters.Spec {
	s := create(o.URL, o.Timeout)
	ts, ok := s.(*tokeninfoSpec)
	if !ok {
		return s
	}

	ts.options = o
	return ts
}

func (s *tokeninfoSpec) Name() string {
	switch s.typ {
	case checkOAuthTokeninfoAnyScopes:
		return OAuthTokeninfoAnyScopeName
	case checkOAuthTokeninfoAllScopes:
		return OAuthTokeninfoAllScopeName
	case checkOAuthTokeninfoAnyKV:
		return OAuthTokeninfoAnyKVName
	case checkOAuthTokeninfoAllKV:
		return OAuthTokeninfoAllKVName
	}
	return AuthUnknown
}

// CreateFilter creates an auth filter. All arguments have to be
// strings. Depending on the variant of the auth tokeninfoFilter, the arguments
// represent scopes or key-value pairs to be checked in the tokeninfo
// response. How scopes or key value pairs are checked is based on the
// type. The shown example for checkOAuthTokeninfoAllScopes will grant
// access only to tokens, that have scopes read-x and write-y:
//
//     s.CreateFilter("read-x", "write-y")
//
func (s *tokeninfoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var ac *authClient
	var ok bool
	if ac, ok = tokeninfoAuthClient[s.options.URL]; !ok {
		ac, err = newAuthClient(s.options.URL, s.options.Timeout, s.options.MaxIdleConns)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}
		tokeninfoAuthClient[s.options.URL] = ac
	}

	f := &tokeninfoFilter{typ: s.typ, authClient: ac, kv: make(map[string][]string)}
	switch f.typ {
	// all scopes
	case checkOAuthTokeninfoAllScopes:
		fallthrough
	case checkOAuthTokeninfoAnyScopes:
		f.scopes = sargs[:]
	// key value pairs
	case checkOAuthTokeninfoAnyKV:
		fallthrough
	case checkOAuthTokeninfoAllKV:
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

// String prints nicely the tokeninfoFilter configuration based on the
// configuration and check used.
func (f *tokeninfoFilter) String() string {
	switch f.typ {
	case checkOAuthTokeninfoAnyScopes:
		return fmt.Sprintf("%s(%s)", OAuthTokeninfoAnyScopeName, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAllScopes:
		return fmt.Sprintf("%s(%s)", OAuthTokeninfoAllScopeName, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAnyKV:
		return fmt.Sprintf("%s(%s)", OAuthTokeninfoAnyKVName, f.kv)
	case checkOAuthTokeninfoAllKV:
		return fmt.Sprintf("%s(%s)", OAuthTokeninfoAllKVName, f.kv)
	}
	return AuthUnknown
}

func (f *tokeninfoFilter) validateAnyScopes(h map[string]interface{}) bool {
	if len(f.scopes) == 0 {
		return true
	}

	vI, ok := h[scopeKey]
	if !ok {
		return false
	}
	v, ok := vI.([]interface{})
	if !ok {
		return false
	}
	var a []string
	for i := range v {
		s, ok := v[i].(string)
		if !ok {
			return false
		}
		a = append(a, s)
	}

	return intersect(f.scopes, a)
}

func (f *tokeninfoFilter) validateAllScopes(h map[string]interface{}) bool {
	if len(f.scopes) == 0 {
		return true
	}

	vI, ok := h[scopeKey]
	if !ok {
		return false
	}
	v, ok := vI.([]interface{})
	if !ok {
		return false
	}
	var a []string
	for i := range v {
		s, ok := v[i].(string)
		if !ok {
			return false
		}
		a = append(a, s)
	}

	return all(f.scopes, a)
}

func (f *tokeninfoFilter) validateAnyKV(h map[string]interface{}) bool {
	for k, v := range f.kv {
		for _, res := range v {
			if v2, ok := h[k].(string); ok {
				if res == v2 {
					return true
				}
			}
		}
	}
	return false
}

func (f *tokeninfoFilter) validateAllKV(h map[string]interface{}) bool {
	if len(h) < len(f.kv) {
		return false
	}
	for k, v := range f.kv {
		for _, res := range v {
			v2, ok := h[k].(string)
			if !ok || res != v2 {
				return false
			}
		}
	}
	return true
}

// Request handles authentication based on the defined auth type.
func (f *tokeninfoFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var authMap map[string]interface{}
	authMapTemp, ok := ctx.StateBag()[tokeninfoCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname(), "")
			return
		}

		var err error
		authMap, err = f.authClient.getTokeninfo(token, ctx)
		if err != nil {
			reason := authServiceAccess
			if err == errInvalidToken {
				reason = invalidToken
			} else {
				log.Errorf("Error while calling tokeninfo: %v.", err)
			}

			unauthorized(ctx, "", reason, f.authClient.url.Hostname(), "")
			return
		}
	} else {
		authMap = authMapTemp.(map[string]interface{})
	}

	uid, _ := authMap[uidKey].(string) // uid can be empty string, but if not we set the who for auditlogging

	var allowed bool
	switch f.typ {
	case checkOAuthTokeninfoAnyScopes:
		allowed = f.validateAnyScopes(authMap)
	case checkOAuthTokeninfoAllScopes:
		allowed = f.validateAllScopes(authMap)
	case checkOAuthTokeninfoAnyKV:
		allowed = f.validateAnyKV(authMap)
	case checkOAuthTokeninfoAllKV:
		allowed = f.validateAllKV(authMap)
	default:
		log.Errorf("Wrong tokeninfoFilter type: %s.", f)
	}

	if !allowed {
		forbidden(ctx, uid, invalidScope, "")
		return
	}

	authorized(ctx, uid)
	ctx.StateBag()[tokeninfoCacheKey] = authMap
}

func (f *tokeninfoFilter) Response(filters.FilterContext) {}

// Close cleans-up the quit channel used for this spec
func (f *tokeninfoFilter) Close() {
	if f.authClient != nil && f.authClient.quit != nil {
		close(f.authClient.quit)
		f.authClient.quit = nil
	}
}
