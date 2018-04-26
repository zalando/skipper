package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	OAuthTokeninfoAnyScopeName = "oauthTokeninfoAnyScope"
	OAuthTokeninfoAllScopeName = "oauthTokeninfoAllScope"
	OAuthTokeninfoAnyKVName    = "oauthTokeninfoAnyKV"
	OAuthTokeninfoAllKVName    = "oauthTokeninfoAllKV"
	AuthUnknown                = "authUnknown"

	authHeaderName      = "Authorization"
	accessTokenQueryKey = "access_token"
	scopeKey            = "scope"
	uidKey              = "uid"
	tokeninfoCacheKey   = "tokeninfo"
)

type (
	authClient struct {
		url *url.URL
	}

	tokeninfoSpec struct {
		typ          roleCheckType
		tokeninfoURL string
		authClient   *authClient
	}

	filter struct {
		typ        roleCheckType
		authClient *authClient
		scopes     []string
		kv         kv
	}
	kv map[string]string
)

var (
	errInvalidAuthorizationHeader = errors.New("invalid authorization header")
	errInvalidToken               = errors.New("invalid token")
)

func getToken(r *http.Request) (string, error) {
	if tok := r.URL.Query().Get(accessTokenQueryKey); tok != "" {
		return tok, nil
	}

	const b = "Bearer "
	h := r.Header.Get(authHeaderName)
	if !strings.HasPrefix(h, b) {
		return "", errInvalidAuthorizationHeader
	}

	return h[len(b):], nil
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
func intersect(left []string, right []string) bool {
	for _, l := range left {
		for _, r := range right {
			if l == r {
				return true
			}
		}
	}

	return false
}

// jsonGet requests url with access token in the URL query specified
// by accessTokenQueryKey, if auth was given and writes into doc.
func jsonGet(url *url.URL, auth string, doc interface{}) error {
	if auth != "" {
		q := url.Query()
		q.Add(accessTokenQueryKey, auth)
		url.RawQuery = q.Encode()
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}

	rsp, err := http.DefaultClient.Do(req)
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

func newAuthClient(baseURL string) (*authClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &authClient{url: u}, nil
}

func (ac *authClient) getTokeninfo(token string) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a)
	return a, err
}

// NewOAuthTokeninfoAllScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all scopes.
func NewOAuthTokeninfoAllScope(OAuthTokeninfoURL string) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAllScopes, tokeninfoURL: OAuthTokeninfoURL}
}

// NewOAuthTokeninfoAnyScope creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one scope.
func NewOAuthTokeninfoAnyScope(OAuthTokeninfoURL string) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAnyScopes, tokeninfoURL: OAuthTokeninfoURL}
}

// NewOAuthTokeninfoAllKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains all key value pairs provided.
func NewOAuthTokeninfoAllKV(OAuthTokeninfoURL string) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAllKV, tokeninfoURL: OAuthTokeninfoURL}
}

// NewOAuthTokeninfoAnyKV creates a new auth filter specification
// to validate authorization for requests. Current implementation uses
// Bearer tokens to authorize requests and checks that the token
// contains at least one key value pair provided.
func NewOAuthTokeninfoAnyKV(OAuthTokeninfoURL string) filters.Spec {
	return &tokeninfoSpec{typ: checkOAuthTokeninfoAnyKV, tokeninfoURL: OAuthTokeninfoURL}
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
// strings. Dependend of the type given arguments are scopes or key
// value pairs to check. How scopes or key value pairs are checked is
// based on the type. The shown example for
// checkOAuthTokeninfoAllScopes will grant access only to tokens,
// that have scopes read-x and write-y:
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

	ac, err := newAuthClient(s.tokeninfoURL)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &filter{typ: s.typ, authClient: ac, kv: make(map[string]string)}
	if len(sargs) > 0 {
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
	}

	return f, nil
}

func (kv kv) String() string {
	var res []string
	for k, v := range kv {
		res = append(res, k, v)
	}
	return strings.Join(res, ",")
}

// String prints nicely the filter configuration based on the
// configuration and check used.
func (f *filter) String() string {
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

func (f *filter) validateAnyScopes(h map[string]interface{}) bool {
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

func (f *filter) validateAllScopes(h map[string]interface{}) bool {
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

func (f *filter) validateAnyKV(h map[string]interface{}) bool {
	for k, v := range f.kv {
		if v2, ok := h[k].(string); ok {
			if v == v2 {
				return true
			}
		}
	}
	return false
}

func (f *filter) validateAllKV(h map[string]interface{}) bool {
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
func (f *filter) Request(ctx filters.FilterContext) {
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
		log.Errorf("Wrong filter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, uid, invalidScope, f.authClient.url.Hostname())
	} else {
		authorized(ctx, uid)
	}
	ctx.StateBag()[tokeninfoCacheKey] = authMap
}

func (f *filter) Response(filters.FilterContext) {}
