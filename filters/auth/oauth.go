package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	logfilter "github.com/zalando/skipper/filters/log"
)

type roleCheckType int

const (
	checkOAuthTokeninfoAnyScopes roleCheckType = iota
	checkOAuthTokeninfoAllScopes
	checkOAuthTokeninfoAnyKV
	checkOAuthTokeninfoAllKV
	checkOAuthTokenintrospectionAnyClaims
	checkOAuthTokenintrospectionAllClaims
	checkOAuthTokenintrospectionAnyKV
	checkOAuthTokenintrospectionAllKV
	checkUnknown
)

type rejectReason string

const (
	missingBearerToken rejectReason = "missing-bearer-token"
	authServiceAccess  rejectReason = "auth-service-access"
	invalidToken       rejectReason = "invalid-token"
	invalidScope       rejectReason = "invalid-scope"
)

const (
	OAuthTokeninfoAnyScopeName           = "oauthTokeninfoAnyScope"
	OAuthTokeninfoAllScopeName           = "oauthTokeninfoAllScope"
	OAuthTokeninfoAnyKVName              = "oauthTokeninfoAnyKV"
	OAuthTokeninfoAllKVName              = "oauthTokeninfoAllKV"
	OAuthTokenintrospectionAnyClaimsName = "oauthTokenintrospectionAnyClaims"
	OAuthTokenintrospectionAllClaimsName = "oauthTokenintrospectionAllClaims"
	OAuthTokenintrospectionAnyKVName     = "oauthTokenintrospectionAnyKV"
	OAuthTokenintrospectionAllKVName     = "oauthTokenintrospectionAllKV"
	AuthUnknown                          = "authUnknown"

	authHeaderName               = "Authorization"
	accessTokenQueryKey          = "access_token"
	scopeKey                     = "scope"
	uidKey                       = "uid"
	tokeninfoCacheKey            = "tokeninfo"
	tokenintrospectionCacheKey   = "tokenintrospection"
	tokenIntrospectionConfigPath = "/.well-known/openid-configuration"
)

type (
	authClient struct {
		url    *url.URL
		client *http.Client
		quit   chan struct{}
	}

	kv map[string]string

	tokeninfoSpec struct {
		typ              roleCheckType
		tokeninfoURL     string
		tokenInfoTimeout time.Duration
		authClient       *authClient
	}

	tokeninfoFilter struct {
		typ        roleCheckType
		authClient *authClient
		scopes     []string
		kv         kv
	}

	tokenIntrospectionSpec struct {
		typ              roleCheckType
		issuerURL        string
		introspectionURL string
		config           *openIDConfig
		authClient       *authClient // TODO(sszuecs): might be different
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

	tokenIntrospectionInfo map[string]interface{}
	// // dynamic keys based on issuer URL
	// managedID       string
	// businessPartner string
	// realm           string
	// tokenType       string

	tokenintrospectFilter struct {
		typ        roleCheckType
		authClient *authClient // TODO(sszuecs): might be different
		claims     []string
		kv         kv
	}
)

var (
	errInvalidAuthorizationHeader    = errors.New("invalid authorization header")
	errInvalidToken                  = errors.New("invalid token")
	errInvalidTokenintrospectionData = errors.New("invalid tokenintrospection data")
)

func (kv kv) String() string {
	var res []string
	for k, v := range kv {
		res = append(res, k, v)
	}
	return strings.Join(res, ",")
}

func getToken(r *http.Request) (string, error) {
	if tok := r.URL.Query().Get(accessTokenQueryKey); tok != "" {
		return tok, nil
	}

	h := r.Header.Get(authHeaderName)
	if !strings.HasPrefix(h, authHeaderPrefix) {
		return "", errInvalidAuthorizationHeader
	}

	return h[len(authHeaderPrefix):], nil
}

func unauthorized(ctx filters.FilterContext, uname string, reason rejectReason, hostname string) {
	ctx.StateBag()[logfilter.AuthUserKey] = uname
	ctx.StateBag()[logfilter.AuthRejectReasonKey] = string(reason)
	rsp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     make(map[string][]string),
	}
	// https://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.2
	rsp.Header.Add("WWW-Authenticate", hostname)
	ctx.Serve(rsp)
}

func authorized(ctx filters.FilterContext, uname string) {
	ctx.StateBag()[logfilter.AuthUserKey] = uname
}

func getStrings(args []interface{}) ([]string, error) {
	s := make([]string, len(args))
	var ok bool
	for i, a := range args {
		s[i], ok = a.(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return s, nil
}

// all checks that all strings in the left are also in the
// right. Right can be a superset of left.
func all(left, right []string) bool {
	for _, l := range left {
		var found bool
		for _, r := range right {
			if l == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// intersect checks that one string in the left is also in the right
func intersect(left, right []string) bool {
	for _, l := range left {
		for _, r := range right {
			if l == r {
				return true
			}
		}
	}

	return false
}

func createHTTPClient(timeout time.Duration, quit chan struct{}) (*http.Client, error) {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
	}

	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				transport.CloseIdleConnections()
			case <-quit:
				return
			}
		}
	}()

	return &http.Client{
		Transport: transport,
	}, nil
}

// jsonGet requests url with access token in the URL query specified
// by accessTokenQueryKey, if auth was given and writes into doc.
func jsonGet(url *url.URL, auth string, doc interface{}, client *http.Client) error {
	if auth != "" {
		q := url.Query()
		q.Set(accessTokenQueryKey, auth)
		url.RawQuery = q.Encode()
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}

	rsp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}

	d := json.NewDecoder(rsp.Body)
	return d.Decode(doc)
}

// jsonPost requests url with access token in the body, if auth was given and writes into doc.
func jsonPost(u *url.URL, auth string, doc *tokenIntrospectionInfo) error {
	if auth == "" {
		return fmt.Errorf("invalid without auth")
	}

	body := url.Values{}
	body.Add("token", auth)

	rsp, err := http.PostForm(u.String(), body)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}
	buf := make([]byte, rsp.ContentLength)
	n, _ := rsp.Body.Read(buf)
	if int64(n) != rsp.ContentLength {
		log.Infof("content-length missmatch body read %d != %d", rsp.ContentLength, n)
	}
	err = json.Unmarshal(buf, &doc)
	if err != nil {
		log.Infof("Failed to unmarshal data: %v", err)
		return err
	}
	return err
}

func newAuthClient(baseURL string, timeout time.Duration) (*authClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	quit := make(chan struct{})
	client, err := createHTTPClient(timeout, quit)
	if err != nil {
		log.Error("Unable to create http client")
	}
	return &authClient{url: u, client: client, quit: quit}, nil
}

func (ac *authClient) getTokeninfo(token string) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a, ac.client)
	return a, err
}

func (ac *authClient) getTokenintrospect(token string) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (tii tokenIntrospectionInfo) Active() bool {
	return tii.getBoolValue("active")
}

func (tii tokenIntrospectionInfo) AuthTime() (time.Time, error) {
	return tii.getUNIXTimeValue("auth_time")
}

func (tii tokenIntrospectionInfo) Azp() (string, error) {
	return tii.getStringValue("azp")
}

func (tii tokenIntrospectionInfo) Exp() (time.Time, error) {
	return tii.getUNIXTimeValue("exp")
}

func (tii tokenIntrospectionInfo) Iat() (time.Time, error) {
	return tii.getUNIXTimeValue("iat")
}

func (tii tokenIntrospectionInfo) Issuer() (string, error) {
	return tii.getStringValue("iss")
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

func (tii tokenIntrospectionInfo) getUNIXTimeValue(k string) (time.Time, error) {
	ts, ok := tii[k].(string)
	if !ok {
		return time.Time{}, errInvalidTokenintrospectionData
	}
	ti, err := strconv.Atoi(ts)
	if err != nil {
		return time.Time{}, errInvalidTokenintrospectionData
	}

	return time.Unix(int64(ti), 0), nil
}

// NewOAuthTokeninfoAllScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all scopes.
func NewOAuthTokeninfoAllScope(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAllScopes, tokeninfoURL: OAuthTokeninfoURL, tokenInfoTimeout: OAuthTokeninfoTimeout}
}

// NewOAuthTokeninfoAnyScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one scope.
func NewOAuthTokeninfoAnyScope(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAnyScopes, tokeninfoURL: OAuthTokeninfoURL, tokenInfoTimeout: OAuthTokeninfoTimeout}
}

// NewOAuthTokeninfoAllKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all key value pairs provided.
func NewOAuthTokeninfoAllKV(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAllKV, tokeninfoURL: OAuthTokeninfoURL, tokenInfoTimeout: OAuthTokeninfoTimeout}
}

// NewOAuthTokeninfoAnyKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one key value pair provided.
func NewOAuthTokeninfoAnyKV(OAuthTokeninfoURL string, OAuthTokeninfoTimeout time.Duration) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAnyKV, tokeninfoURL: OAuthTokeninfoURL, tokenInfoTimeout: OAuthTokeninfoTimeout}
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
//     s.CreateFilter(read-x", "write-y")
//
func (s *tokeninfoSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	ac, err := newAuthClient(s.tokeninfoURL, s.tokenInfoTimeout)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &tokeninfoFilter{typ: s.typ, authClient: ac, kv: make(map[string]string)}
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
		if v2, ok := h[k].(string); ok {
			if v == v2 {
				return true
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
		v2, ok := h[k].(string)
		if !ok || v != v2 {
			return false
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
		token, err := getToken(r)
		if err != nil {
			unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname())
			return
		}
		if token == "" {
			unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname())
			return
		}

		authMap, err = f.authClient.getTokeninfo(token)
		if err != nil {
			reason := authServiceAccess
			if err == errInvalidToken {
				reason = invalidToken
			}
			unauthorized(ctx, "", reason, f.authClient.url.Hostname())
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
		log.Errorf("Wrong tokeninfoFilter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, uid, invalidScope, f.authClient.url.Hostname())
	} else {
		authorized(ctx, uid)
	}
	ctx.StateBag()[tokeninfoCacheKey] = authMap
}

// Close cleans-up the quit channel used for this spec
func (f *tokeninfoFilter) Close() {
	if f.authClient.quit != nil {
		close(f.authClient.quit)
	}
}

func (f *tokeninfoFilter) Response(filters.FilterContext) {}

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
func NewOAuthTokenintrospectionAnyKV(oauthIssuerURL, oauthIntrospectionURL string) filters.Spec {
	cfg, err := getOpenIDConfig(oauthIssuerURL)
	if err != nil {
		return &tokenIntrospectionSpec{
			typ:              checkOAuthTokenintrospectionAnyKV,
			issuerURL:        oauthIssuerURL,
			introspectionURL: oauthIntrospectionURL,
		}
	}

	if oauthIntrospectionURL != "" {
		cfg.IntrospectionEndpoint = oauthIntrospectionURL
	}
	return &tokenIntrospectionSpec{
		typ:              checkOAuthTokenintrospectionAnyKV,
		issuerURL:        oauthIssuerURL,
		introspectionURL: cfg.IntrospectionEndpoint,
	}
}

func getOpenIDConfig(issuerURL string) (*openIDConfig, error) {
	u, err := url.Parse(issuerURL + "/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}

	var cfg openIDConfig
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
	if len(sargs) == 0 || s.introspectionURL == "" {
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
	// all claims
	case checkOAuthTokenintrospectionAllClaims:
		fallthrough
	case checkOAuthTokenintrospectionAnyClaims:
		f.claims = sargs[:]
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

func (f *tokenintrospectFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info tokenIntrospectionInfo
	authMapTemp, ok := ctx.StateBag()[tokenintrospectionCacheKey]
	if !ok {
		token, err := getToken(r)
		if err != nil {
			unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname())
			return
		}
		if token == "" {
			unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname())
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
		info = authMapTemp.(tokenIntrospectionInfo)
	}
	log.Infof("info: %#v", info)
	log.Infof("info.Active(): %v", info.Active())
	s, err := info.Issuer()
	log.Infof("info.Issuer(): %s %v", s, err)
}
func (f *tokenintrospectFilter) Response(filters.FilterContext) {}
