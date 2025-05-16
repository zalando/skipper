package auth

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/metrics"
)

const (
	// Deprecated, use filters.OAuthTokeninfoAnyScopeName instead
	OAuthTokeninfoAnyScopeName = filters.OAuthTokeninfoAnyScopeName
	// Deprecated, use filters.OAuthTokeninfoAllScopeName instead
	OAuthTokeninfoAllScopeName = filters.OAuthTokeninfoAllScopeName
	// Deprecated, use filters.OAuthTokeninfoAnyKVName instead
	OAuthTokeninfoAnyKVName = filters.OAuthTokeninfoAnyKVName
	// Deprecated, use filters.OAuthTokeninfoAllKVName instead
	OAuthTokeninfoAllKVName = filters.OAuthTokeninfoAllKVName

	tokeninfoCacheKey = "tokeninfo"
)

type TokeninfoOptions struct {
	URL          string
	Timeout      time.Duration
	MaxIdleConns int
	Tracer       opentracing.Tracer
	Metrics      metrics.Metrics

	// CacheSize configures the maximum number of cached tokens.
	// The cache periodically evicts random items when number of cached tokens exceeds CacheSize.
	// Zero value disables tokeninfo cache.
	CacheSize int

	// CacheTTL limits the lifetime of a cached tokeninfo.
	// Tokeninfo is cached for the duration of "expires_in" field value seconds or
	// for the duration of CacheTTL if it is not zero and less than "expires_in" value.
	CacheTTL time.Duration
}

type (
	tokeninfoSpec struct {
		typ     roleCheckType
		options TokeninfoOptions

		tokeninfoValidateYamlConfigParser *yamlConfigParser[tokeninfoValidateFilterConfig]
	}

	tokeninfoFilter struct {
		typ    roleCheckType
		client tokeninfoClient
		scopes []string
		kv     kv
	}

	tokeninfoValidateFilter struct {
		client tokeninfoClient
		config *tokeninfoValidateFilterConfig
	}

	// tokeninfoValidateFilterConfig implements [yamlConfig],
	// make sure it is not modified after initialization.
	tokeninfoValidateFilterConfig struct {
		OptOutAnnotations    []string `json:"optOutAnnotations,omitempty"`
		UnauthorizedResponse string   `json:"unauthorizedResponse,omitempty"`
		OptOutHosts          []string `json:"optOutHosts,omitempty"`

		optOutHostsCompiled []*regexp.Regexp
	}
)

var tokeninfoAuthClient map[string]tokeninfoClient = make(map[string]tokeninfoClient)

// getTokeninfoClient creates new or returns a cached instance of tokeninfoClient
func (o *TokeninfoOptions) getTokeninfoClient() (tokeninfoClient, error) {
	if c, ok := tokeninfoAuthClient[o.URL]; ok {
		return c, nil
	}

	c, err := o.newTokeninfoClient()
	if err == nil {
		tokeninfoAuthClient[o.URL] = c
	}
	return c, err
}

// newTokeninfoClient creates new instance of tokeninfoClient
func (o *TokeninfoOptions) newTokeninfoClient() (tokeninfoClient, error) {
	var c tokeninfoClient

	c, err := newAuthClient(o.URL, tokenInfoSpanName, o.Timeout, o.MaxIdleConns, o.Tracer, false)
	if err != nil {
		return nil, err
	}

	if o.CacheSize > 0 {
		c = newTokeninfoCache(c, o.Metrics, o.CacheSize, o.CacheTTL)
	}
	return c, nil
}

func NewOAuthTokeninfoAllScopeWithOptions(to TokeninfoOptions) filters.Spec {
	return &tokeninfoSpec{
		typ:     checkOAuthTokeninfoAllScopes,
		options: to,
	}
}

// NewOAuthTokeninfoAllScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all scopes.
func NewOAuthTokeninfoAllScope(oauthTokeninfoURL string, oauthTokeninfoTimeout time.Duration) filters.Spec {
	return NewOAuthTokeninfoAllScopeWithOptions(TokeninfoOptions{
		URL:     oauthTokeninfoURL,
		Timeout: oauthTokeninfoTimeout,
	})
}

func NewOAuthTokeninfoAnyScopeWithOptions(to TokeninfoOptions) filters.Spec {
	return &tokeninfoSpec{
		typ:     checkOAuthTokeninfoAnyScopes,
		options: to,
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

func NewOAuthTokeninfoAllKVWithOptions(to TokeninfoOptions) filters.Spec {
	return &tokeninfoSpec{
		typ:     checkOAuthTokeninfoAllKV,
		options: to,
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

func NewOAuthTokeninfoAnyKVWithOptions(to TokeninfoOptions) filters.Spec {
	return &tokeninfoSpec{
		typ:     checkOAuthTokeninfoAnyKV,
		options: to,
	}
}

func NewOAuthTokeninfoValidate(to TokeninfoOptions) filters.Spec {
	p := newYamlConfigParser[tokeninfoValidateFilterConfig](64)
	return &tokeninfoSpec{
		typ:     checkOAuthTokeninfoValidate,
		options: to,

		tokeninfoValidateYamlConfigParser: &p,
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
		return filters.OAuthTokeninfoAnyScopeName
	case checkOAuthTokeninfoAllScopes:
		return filters.OAuthTokeninfoAllScopeName
	case checkOAuthTokeninfoAnyKV:
		return filters.OAuthTokeninfoAnyKVName
	case checkOAuthTokeninfoAllKV:
		return filters.OAuthTokeninfoAllKVName
	case checkOAuthTokeninfoValidate:
		return filters.OAuthTokeninfoValidateName
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
//	s.CreateFilter("read-x", "write-y")
func (s *tokeninfoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	ac, err := s.options.getTokeninfoClient()
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	if s.typ == checkOAuthTokeninfoValidate {
		config, err := s.tokeninfoValidateYamlConfigParser.parseSingleArg(args)
		if err != nil {
			return nil, err
		}
		return &tokeninfoValidateFilter{client: ac, config: config}, nil
	}

	f := &tokeninfoFilter{typ: s.typ, client: ac, kv: make(map[string][]string)}
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
		return fmt.Sprintf("%s(%s)", filters.OAuthTokeninfoAnyScopeName, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAllScopes:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokeninfoAllScopeName, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAnyKV:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokeninfoAnyKVName, f.kv)
	case checkOAuthTokeninfoAllKV:
		return fmt.Sprintf("%s(%s)", filters.OAuthTokeninfoAllKVName, f.kv)
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

	for _, scope := range f.scopes {
		if contains(v, scope) {
			return true
		}
	}
	return false
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

	for _, scope := range f.scopes {
		if !contains(v, scope) {
			return false
		}
	}
	return true
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

func contains(vals []interface{}, s string) bool {
	for _, v := range vals {
		if v == s {
			return true
		}
	}
	return false
}

// Request handles authentication based on the defined auth type.
func (f *tokeninfoFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var authMap map[string]interface{}
	authMapTemp, ok := ctx.StateBag()[tokeninfoCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingBearerToken, "", "")
			return
		}

		var err error
		authMap, err = f.client.getTokeninfo(token, ctx)
		if err != nil {
			reason := authServiceAccess
			if err == errInvalidToken {
				reason = invalidToken
			} else {
				ctx.Logger().Errorf("Error while calling tokeninfo: %v", err)
			}

			unauthorized(ctx, "", reason, "", "")
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
		ctx.Logger().Errorf("Wrong tokeninfoFilter type: %s.", f)
	}

	if !allowed {
		forbidden(ctx, uid, invalidScope, "")
		return
	}

	authorized(ctx, uid)
	ctx.StateBag()[tokeninfoCacheKey] = authMap
}

func (f *tokeninfoFilter) Response(filters.FilterContext) {}

func (c *tokeninfoValidateFilterConfig) initialize() error {
	for _, host := range c.OptOutHosts {
		if r, err := regexp.Compile(host); err != nil {
			return fmt.Errorf("failed to compile opt-out host pattern: %q", host)
		} else {
			c.optOutHostsCompiled = append(c.optOutHostsCompiled, r)
		}
	}
	return nil
}

func (f *tokeninfoValidateFilter) Request(ctx filters.FilterContext) {
	if _, ok := ctx.StateBag()[tokeninfoCacheKey]; ok {
		return // tokeninfo was already validated by a preceding filter
	}

	if len(f.config.OptOutAnnotations) > 0 {
		annotations := annotate.GetAnnotations(ctx)
		for _, annotation := range f.config.OptOutAnnotations {
			if _, ok := annotations[annotation]; ok {
				return // opt-out from validation
			}
		}
	}

	if len(f.config.optOutHostsCompiled) > 0 {
		host := ctx.Request().Host
		for _, r := range f.config.optOutHostsCompiled {
			if r.MatchString(host) {
				return // opt-out from validation
			}
		}
	}

	token, ok := getToken(ctx.Request())
	if !ok {
		f.serveUnauthorized(ctx)
		return
	}

	tokeninfo, err := f.client.getTokeninfo(token, ctx)
	if err != nil {
		f.serveUnauthorized(ctx)
		return
	}

	uid, _ := tokeninfo[uidKey].(string)
	authorized(ctx, uid)
	ctx.StateBag()[tokeninfoCacheKey] = tokeninfo
}

func (f *tokeninfoValidateFilter) serveUnauthorized(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"Content-Length": []string{strconv.Itoa(len(f.config.UnauthorizedResponse))},
		},
		Body: io.NopCloser(strings.NewReader(f.config.UnauthorizedResponse)),
	})
}

func (f *tokeninfoValidateFilter) Response(filters.FilterContext) {}
