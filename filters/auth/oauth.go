package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	logfilter "github.com/zalando/skipper/filters/log"
	"golang.org/x/oauth2"
)

type roleCheckType int

const (
	checkAnyScopes roleCheckType = iota
	checkAllScopes
	checkAnyKV
	checkAllKV
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

// TODO: discuss these names, because these are the filter names used by the endusers
const (
	AuthAnyScopeName = "authAnyScope"
	AuthAllScopeName = "authAllScope"
	AuthAnyKVName    = "authAnyKV"
	AuthAllKVName    = "authAllKV"
	AuthUnknown      = "authUnknown"
	// BasicAuthAnyScopeName = "basicAuth"

	authHeaderName = "Authorization"
	realmKey       = "realm"
	scopeKey       = "scope"
	uidKey         = "uid"
)

type (
	authClient struct{ urlBase string }

	spec struct {
		typ        roleCheckType
		cfg        oauth2.Config
		authClient *authClient
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
)

func getToken(r *http.Request) (string, error) {
	const b = "Bearer "
	h := r.Header.Get(authHeaderName)
	if !strings.HasPrefix(h, b) {
		return "", errInvalidAuthorizationHeader
	}

	return h[len(b):], nil
}

func unauthorized(ctx filters.FilterContext, uname string, reason rejectReason) {
	ctx.StateBag()[logfilter.AuthUserKey] = uname
	ctx.StateBag()[logfilter.AuthRejectReasonKey] = string(reason)
	ctx.Serve(&http.Response{StatusCode: http.StatusUnauthorized})
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
	if len(left) > len(right) {
		return false
	}

	for _, l := range left {
		for _, r := range right {
			if l != r {
				return false
			}
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

// jsonGet requests url with Bearer auth header if `auth` was given
// and writes into doc.
func jsonGet(url, auth string, doc interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if auth != "" {
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

func (ac *authClient) getTokeninfo(token string) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.urlBase, token, &a)
	return a, err
}

func newSpec(typ roleCheckType, cfg oauth2.Config) filters.Spec {
	return &spec{typ: typ, cfg: cfg}
}

// Options to configure auth providers
type Options struct {
	// TokenURL is the tokeninfo URL able to return information
	// about a token.
	TokenURL string
	// AuthType is the type of authnz function you want to
	// use. Examples are the values "authAnyScope" or "authAllScope",
	// defined in constants AuthAnyScopeName and AuthAllScopeName.
	AuthType string
}

// NewAuth creates a new auth filter specification to validate
// authorization for requests. Current implementation uses Bearer
// tokens to authorize requests, optionally check realms and
// optionally check scopes.
func NewAuth(o Options) filters.Spec {
	cfg := oauth2.Config{
		Endpoint: oauth2.Endpoint{
			TokenURL: o.TokenURL,
		},
	}

	return newSpec(typeForName(o.AuthType), cfg)
}

func typeForName(s string) roleCheckType {
	switch s {
	case AuthAllScopeName:
		return checkAllScopes
	case AuthAnyScopeName:
		return checkAnyScopes
	case AuthAnyKVName:
		return checkAnyKV
	case AuthAllKVName:
		return checkAllKV
	}
	return checkUnknown
}

func (s *spec) Name() string {
	switch s.typ {
	case checkAnyScopes:
		return AuthAnyScopeName
	case checkAllScopes:
		return AuthAllScopeName
	case checkAnyKV:
		return AuthAnyKVName
	case checkAllKV:
		return AuthAllKVName
	}
	return AuthUnknown
}

// CreateFilter creates an auth filter. All arguments have to be
// strings. The first argument is the realm and the rest the given
// scopes to check. How scopes are checked is based on the type. The
// shown example will grant access only to tokens from `myrealm`, that
// have scopes `read-x` and `write-y`:
//
//     s.CreateFilter("myrealm", "read-x", "write-y")
//
func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	if s.typ == checkUnknown {
		return nil, filters.ErrInvalidFilterParameters
	}

	ac := &authClient{urlBase: s.cfg.Endpoint.TokenURL}

	f := &filter{typ: s.typ, authClient: ac, kv: make(map[string]string)}
	if len(sargs) > 0 {
		switch f.typ {
		case checkAnyKV:
			fallthrough
		case checkAllKV:
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
	case checkAnyScopes:
		return fmt.Sprintf("%s(%s,%s)", AuthAnyScopeName, f.realm, strings.Join(f.scopes, ","))
	case checkAllScopes:
		return fmt.Sprintf("%s(%s,%s)", AuthAllScopeName, f.realm, strings.Join(f.scopes, ","))
	case checkAnyKV:
		return fmt.Sprintf("%s(%s,%s)", AuthAnyKVName, f.realm, f.kv)
	case checkAllKV:
		return fmt.Sprintf("%s(%s,%s)", AuthAllKVName, f.realm, f.kv)
	}
	return AuthUnknown
}

func (f *filter) validateRealm(h map[string]interface{}) bool {
	if f.realm == "" {
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
		a = append(a, v[i].(string))
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
		a = append(a, v[i].(string))
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

	token, err := getToken(r)
	if err != nil {
		unauthorized(ctx, "", missingBearerToken)
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
		unauthorized(ctx, "", reason)
		return
	}

	uid, ok := authMap[uidKey].(string)
	if !ok || !f.validateRealm(authMap) {
		unauthorized(ctx, uid, invalidRealm)
		return
	}

	var allowed bool
	switch f.typ {
	case checkAnyScopes:
		allowed = f.validateAnyScopes(authMap)
	case checkAllScopes:
		allowed = f.validateAllScopes(authMap)
	case checkAnyKV:
		allowed = f.validateAnyKV(authMap)
	case checkAllKV:
		allowed = f.validateAllKV(authMap)
	default:
		log.Errorf("Wrong filter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, uid, invalidScope)
	} else {

		authorized(ctx, uid)
	}
}

func (f *filter) Response(filters.FilterContext) {}
