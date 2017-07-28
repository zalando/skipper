package auth

import (
	// "encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	logfilter "github.com/zalando/skipper/filters/log"
)

const authHeaderName = "Authorization"

type roleCheckType int

const (
	checkScope roleCheckType = iota
	checkGroup
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

const (
	AuthName      = "auth"
	AuthGroupName = "authGroup"
	// BasicAuthName = "basicAuth"
)

type (
	authClient  struct{ urlBase string }
	groupClient struct{ urlBase string }

	authDoc struct {
		Uid    string   `json:"uid"`
		Realm  string   `json:"realm"`
		Scopes []string `json:"scope"` // TODO: verify this with service2service authentication
	}

	groupDoc struct {
		Id string `json:"id"`
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
		args        []string
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
		ts[i] = ti.Id
	}

	return ts, nil
}

func newSpec(typ roleCheckType) filters.Spec {
	return &spec{typ: typ}
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
//
func NewAuth() filters.Spec {
	return newSpec(checkScope)
}

// NewAuditGroup reates a new auth filter specification to validate authorization
// tokens, optionally check realms and optionally check groups.
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
//
func NewAuthGroup() filters.Spec {
	return newSpec(checkGroup)
}

func (s *spec) Name() string {
	if s.typ == checkScope {
		return AuthName
	} else {
		return AuthGroupName
	}
}

func (s *spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var ac *authClient
	ac, sargs = &authClient{sargs[0]}, sargs[1:]

	var tc *groupClient
	if s.typ == checkGroup {
		tc, sargs = &groupClient{sargs[0]}, sargs[1:]
	}

	f := &filter{typ: s.typ, authClient: ac, groupClient: tc}
	if len(sargs) > 0 {
		f.realm, f.args = sargs[0], sargs[1:]
	}

	return f, nil

}

func (f *filter) validateRealm(a *authDoc) bool {
	if f.realm == "" {
		return true
	}

	return a.Realm == f.realm
}

func (f *filter) validateScope(a *authDoc) bool {
	if len(f.args) == 0 {
		return true
	}

	return intersect(f.args, a.Scopes)
}

func (f *filter) validateGroup(token string, a *authDoc) (bool, error) {
	if len(f.args) == 0 {
		return true, nil
	}

	groups, err := f.groupClient.getGroups(a.Uid, token)
	return intersect(f.args, groups), err
}

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
		unauthorized(ctx, authDoc.Uid, invalidRealm)
		return
	}

	if f.typ == checkScope {
		if !f.validateScope(authDoc) {
			unauthorized(ctx, authDoc.Uid, invalidScope)
			return
		}

		authorized(ctx, authDoc.Uid)
		return
	}

	if valid, err := f.validateGroup(token, authDoc); err != nil {
		unauthorized(ctx, authDoc.Uid, groupServiceAccess)
		log.Println(err)
	} else if !valid {
		unauthorized(ctx, authDoc.Uid, invalidGroup)
	} else {
		authorized(ctx, authDoc.Uid)
	}
}

func (f *filter) Response(_ filters.FilterContext) {}
