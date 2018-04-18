package auth

import (
	// "encoding/base64"
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

var (
	// oauth2Config is the global config, which is used as default
	// to configure all oauth2 related filters.
	oauth2Config oauth2.Config
)

type roleCheckType int

const (
	// set operation any()
	checkAnyScopes roleCheckType = iota
	checkAllScopes               // set operation all()
	checkUID                     // TODO(sszuecs): should be syntactic sugar to work on 'uid' scopes
	checkGroup                   // TODO: define an interface: external webhook API + json/type
)

type rejectReason string

const (
	missingBearerToken rejectReason = "missing-bearer-token"
	authServiceAccess  rejectReason = "auth-service-access"
	invalidToken       rejectReason = "invalid-token"
	invalidRealm       rejectReason = "invalid-realm"
	invalidScope       rejectReason = "invalid-scope"
	groupServiceAccess rejectReason = "group-service-access"
	invalidGroup       rejectReason = "invalid-group"
)

// TODO: discuss these names, because these are the filter names used by the endusers
const (
	AuthAnyName   = "authAny"
	AuthAllName   = "authAll"
	AuthUIDName   = "authUid"
	AuthGroupName = "authGroup"
	AuthUnknown   = "authUnknown"
	// BasicAuthAnyName = "basicAuth"

	authHeaderName = "Authorization"
)

type (
	authClient  struct{ urlBase string }
	groupClient struct{ urlBase string }

	authDoc struct {
		UID    string   `json:"uid"`
		Realm  string   `json:"realm"`
		Scopes []string `json:"scope"`
	}

	groupDoc struct {
		ID string `json:"id"`
	}

	spec struct {
		typ        roleCheckType
		authClient *authClient
	}

	filter struct {
		typ         roleCheckType
		authClient  *authClient
		groupClient *groupClient
		realm       string
		scopes      []string
	}
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
	ctx.StateBag()["auth-user"] = uname
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
		for _, r := range right {
			if l != r {
				return false
			}
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

func (ac *authClient) validate(token string) (*authDoc, error) {
	var a authDoc
	err := jsonGet(ac.urlBase, token, &a)
	return &a, err
}

func (tc *groupClient) getGroups(uid, token string) ([]string, error) {
	var t []groupDoc
	err := jsonGet(tc.urlBase+uid, token, &t)
	if err != nil {
		return nil, err
	}

	ts := make([]string, len(t))
	for i, ti := range t {
		ts[i] = ti.ID
	}

	return ts, nil
}

func newSpec(typ roleCheckType) filters.Spec {
	return &spec{typ: typ}
}

// Options to configure auth providers
type Options struct {
	// TokenURL is the tokeninfo URL able to return information
	// about a token.
	TokenURL string
}

// NewOAuth2 creates a new auth filter specification to validate
// authorization tokens.
func NewOAuth2(o Options) filters.Spec {
	oauth2Config = oauth2.Config{
		Endpoint: oauth2.Endpoint{
			TokenURL: o.TokenURL,
		},
	}

	return newSpec(checkAnyScopes)
}

// NewAuth creates a new auth filter specification to validate authorization
// tokens, optionally check realms and optionally check scopes.
//
// authUrlBase: the url of the token validation service.
// The filter expects the service to validate the token found in the
// Authorization header and in case of a valid token, it expects it
// to return the user id and the realm of the user associated with
// the token ('uid' and 'realm' fields in the returned json document).
// The token is set as the Authorization Bearer header.
func NewAuth() filters.Spec {
	return newSpec(checkAnyScopes)
}

// NewAuthGroup creates a new auth filter specification to validate
// authorization tokens, optionally check realms and optionally check
// groups.
//
// authUrlBase: the url of the token validation service. The filter
// expects the service to validate the token found in the Authorization
// header and in case of a valid token, it expects it to return the
// user id and the realm of the user associated with the token ('uid'
// and 'realm' fields in the returned json document). The token is set
// as the Authorization Bearer header.
//
// groupUrlBase: this service is queried for the group ids, that the
// user is a member of ('id' field of the returned json document's
// items). The user id of the user is appended at the end of the url.
func NewAuthGroup() filters.Spec {
	return newSpec(checkGroup)
}

func (s *spec) Name() string {
	switch s.typ {
	case checkAnyScopes:
		return AuthAnyName
	case checkAllScopes:
		return AuthAllName
	case checkUID:
		return AuthUIDName
	case checkGroup:
		return AuthGroupName
	}
	return AuthUnknown
}

// CreateFilter creates an auth filter. All arguments have to be
// strings. The first argument relates to the type of the authnz
// check, the second argument is the realm and the rest the given
// scopes to check. How scopes are checked is based on the type. The
// shown example will grant access only to tokens from `myrealm`, that
// have scopes `read-x` and `write-y`:
//
//     s.CreateFilter("authAll", "myrealm", "read-x", "write-y")
func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var ac *authClient
	var tc *groupClient

	switch sargs[0] {
	case AuthAnyName:
		ac = &authClient{sargs[0]}
	case AuthAllName:
		ac = &authClient{sargs[0]}
	case AuthUIDName:
		ac = &authClient{sargs[0]}
	case AuthGroupName:
		tc = &groupClient{sargs[0]}
	}

	sargs = sargs[1:]

	f := &filter{typ: s.typ, authClient: ac, groupClient: tc}
	if len(sargs) > 0 {
		f.realm, f.scopes = sargs[0], sargs[1:]
	}

	return f, nil

}

// String prints nicely the filter configuration based on the
// configuration and check used.
func (f *filter) String() string {
	switch f.typ {
	case checkAnyScopes:
		return fmt.Sprintf("%s(%s,%s)", AuthAnyName, f.realm, strings.Join(f.scopes, ","))
	case checkAllScopes:
		return fmt.Sprintf("%s(%s,%s)", AuthAllName, f.realm, strings.Join(f.scopes, ","))
	case checkUID:
		return fmt.Sprintf("%s(%s,%s)", AuthUIDName, f.realm, strings.Join(f.scopes, ","))
	case checkGroup:
		return fmt.Sprintf("%s(%s,%s)", AuthGroupName, f.realm, strings.Join(f.scopes, ","))
	}
	return AuthUnknown
}

func (f *filter) validateRealm(a *authDoc) bool {
	if f.realm == "" {
		return true
	}

	return a.Realm == f.realm
}

func (f *filter) validateAnyScopes(a *authDoc) bool {
	if len(f.scopes) == 0 {
		return true
	}

	return intersect(f.scopes, a.Scopes)
}

func (f *filter) validateAllScopes(a *authDoc) bool {
	if len(f.scopes) == 0 {
		return true
	}

	return all(f.scopes, a.Scopes)
}

func (f *filter) validateGroup(token string, a *authDoc) (bool, error) {
	if len(f.scopes) == 0 {
		return true, nil
	}

	groups, err := f.groupClient.getGroups(a.UID, token)
	return intersect(f.scopes, groups), err
}

// Request handles authentication based on the defined auth type.
func (f *filter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	token, err := getToken(r)
	if err != nil {
		unauthorized(ctx, "", missingBearerToken)
		return
	}

	authDoc, err := f.authClient.validate(token)
	if err != nil {
		reason := authServiceAccess
		if err == errInvalidToken {
			reason = invalidToken
		} else {
			log.Println(err)
		}

		unauthorized(ctx, "", reason)
		return
	}

	if !f.validateRealm(authDoc) {
		unauthorized(ctx, authDoc.UID, invalidRealm)
		return
	}

	var allowed bool
	switch f.typ {
	case checkAnyScopes:
		allowed = f.validateAnyScopes(authDoc)
	case checkAllScopes:
		allowed = f.validateAllScopes(authDoc)
	case checkUID:
		// TODO
	case checkGroup:
		if valid, err := f.validateGroup(token, authDoc); err != nil {
			unauthorized(ctx, authDoc.UID, groupServiceAccess)
			log.Println(err)
		} else if !valid {
			unauthorized(ctx, authDoc.UID, invalidGroup)
		} else {
			authorized(ctx, authDoc.UID)
		}
		return
	default:
		log.Errorf("Wrong filter type: %s", f)
	}

	if !allowed {
		unauthorized(ctx, authDoc.UID, invalidScope)
	} else {

		authorized(ctx, authDoc.UID)
	}
}

func (f *filter) Response(filters.FilterContext) {}
