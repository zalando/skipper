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
	checkOAuthTokeninfoRealmAnyScopes
	checkOAuthTokeninfoRealmAllScopes
	checkOAuthTokeninfoRealmAnyKV
	checkOAuthTokeninfoRealmAllKV
	checkUnknown
)

type rejectReason string

const (
	missingBearerToken rejectReason = "missing-bearer-token"
	authServiceAccess  rejectReason = "auth-service-access"
	invalidToken       rejectReason = "invalid-token"
	invalidRealm       rejectReason = "invalid-realm"
	invalidScope       rejectReason = "invalid-scope"
)

const (
	OAuthTokeninfoAnyScopeName      = "oauthTokeninfoAnyScope"
	OAuthTokeninfoAllScopeName      = "oauthTokeninfoAllScope"
	OAuthTokeninfoAnyKVName         = "oauthTokeninfoAnyKV"
	OAuthTokeninfoAllKVName         = "oauthTokeninfoAllKV"
	OAuthTokeninfoRealmAnyScopeName = "oauthTokeninfoRealmAnyScope"
	OAuthTokeninfoRealmAllScopeName = "oauthTokeninfoRealmAllScope"
	OAuthTokeninfoRealmAnyKVName    = "oauthTokeninfoRealmAnyKV"
	OAuthTokeninfoRealmAllKVName    = "oauthTokeninfoRealmAllKV"
	AuthUnknown                     = "authUnknown"

	authHeaderName = "Authorization"
	realmKey       = "realm" // TODO(sszuecs): should be a parameter to a filter
	scopeKey       = "scope"
	uidKey         = "uid"
)

type (
	// TODO(sszuecs) cleanup comment
	// We have to have 2 kind of URLs, based on tokeninfo vs. token_introspection
	// tokeninfo (has to be set by ENV or CLI):
	//    zalando: http://localhost:9021/oauth2/tokeninfo?access_token=accessToken
	//    google : https://www.googleapis.com/oauth2/v1/tokeninfo?access_token=accessToken
	// token_introspction (needs an issue https://identity.zalando.com , which can then be called to /.well-known/openid-configuration, returning OPTIONAL the introspection_endpoint https://tools.ietf.org/html/draft-ietf-oauth-discovery-06
	//    zalando: curl -X POST -d "token=$(ztoken)" localhost:9021/oauth2/introspect

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
		realm      string
		scopes     []string
		kv         kv
	}
	kv map[string]string
)

var (
	errInvalidAuthorizationHeader = errors.New("invalid authorization header")
	errInvalidToken               = errors.New("invalid token")

	hasRealmCheck = map[roleCheckType]bool{
		checkOAuthTokeninfoRealmAnyScopes: true,
		checkOAuthTokeninfoRealmAllScopes: true,
		checkOAuthTokeninfoRealmAnyKV:     true,
		checkOAuthTokeninfoRealmAllKV:     true,
	}
)

func getToken(r *http.Request) (string, error) {
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

// jsonGet requests url with Bearer auth header if auth was given
// and writes into doc.
func jsonGet(url, auth string, doc interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if auth != "" {
		// TODO(sszuecs): set query string instead and change godoc
		req.Header.Set(authHeaderName, "Bearer "+auth)
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
	err := jsonGet(ac.url.String(), token, &a)
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

// TODO(sszuecs): add realm check type constructor functions

func (s *tokeninfoSpec) Name() string {
	switch s.typ {
	// TODO(sszuecs): add realm check types
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
// strings. The first argument is the realm and the rest the given
// scopes to check. How scopes are checked is based on the type. The
// shown example will grant access only to tokens from myrealm, that
// have scopes read-x and write-y:
//
//     s.CreateFilter("myrealm", "read-x", "write-y")
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
		// TODO(sszuecs): add realm check types
		case checkOAuthTokeninfoAnyKV:
			fallthrough
		case checkOAuthTokeninfoAllKV:
			f.realm = sargs[0]
			sargs = sargs[1:]
			for i := 0; i+1 < len(sargs); i += 2 {
				f.kv[sargs[i]] = sargs[i+1]
			}
			if len(sargs) == 0 || len(sargs)%2 != 0 {
				return nil, filters.ErrInvalidFilterParameters
			}
		default:
			f.realm, f.scopes = sargs[0], sargs[1:]
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
	// TODO(sszuecs): add realm check types
	case checkOAuthTokeninfoAnyScopes:
		return fmt.Sprintf("%s(%s,%s)", OAuthTokeninfoAnyScopeName, f.realm, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAllScopes:
		return fmt.Sprintf("%s(%s,%s)", OAuthTokeninfoAllScopeName, f.realm, strings.Join(f.scopes, ","))
	case checkOAuthTokeninfoAnyKV:
		return fmt.Sprintf("%s(%s,%s)", OAuthTokeninfoAnyKVName, f.realm, f.kv)
	case checkOAuthTokeninfoAllKV:
		return fmt.Sprintf("%s(%s,%s)", OAuthTokeninfoAllKVName, f.realm, f.kv)
	}
	return AuthUnknown
}

func (f *filter) validateRealm(h map[string]interface{}) bool {
	if f.realm == "" {
		return true
	}
	if _, ok := hasRealmCheck[f.typ]; !ok {
		return true
	}

	vI, ok := h[realmKey]
	if !ok {
		return false
	}
	v, ok := vI.(string)
	if !ok {
		return false
	}
	return v == f.realm
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

// TODO(sszuecs): add realm check validate filter methods

// Request handles authentication based on the defined auth type.
func (f *filter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	token, err := getToken(r)
	if err != nil {
		unauthorized(ctx, "", missingBearerToken, f.authClient.url.Hostname())
		return
	}

	authMap, err := f.authClient.getTokeninfo(token)
	if err != nil {
		reason := authServiceAccess
		if err == errInvalidToken {
			reason = invalidToken
		} else {
			log.Errorf("Failed to get token: %v", err)
		}
		unauthorized(ctx, "", reason, f.authClient.url.Hostname())
		return
	}

	uid, ok := authMap[uidKey].(string)
	if !ok || !f.validateRealm(authMap) {
		unauthorized(ctx, uid, invalidRealm, f.authClient.url.Hostname())
		return
	}

	var allowed bool
	switch f.typ {
	// TODO(sszuecs): add realm check types
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
}

func (f *filter) Response(filters.FilterContext) {}
