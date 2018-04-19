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
	// set operation any()
	checkAnyScopes roleCheckType = iota
	checkAllScopes               // set operation all()
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
	AuthAnyName = "authAny"
	AuthAllName = "authAll"
	AuthUnknown = "authUnknown"
	// BasicAuthAnyName = "basicAuth"

	authHeaderName = "Authorization"
)

type (
	authClient struct{ urlBase string }

	authDoc struct {
		UID    string   `json:"uid"`
		Realm  string   `json:"realm"`
		Scopes []string `json:"scope"`
	}

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

func newSpec(typ roleCheckType, cfg oauth2.Config) filters.Spec {
	return &spec{typ: typ, cfg: cfg}
}

// Options to configure auth providers
type Options struct {
	// TokenURL is the tokeninfo URL able to return information
	// about a token.
	TokenURL string
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
	case AuthAllName:
		return checkAllScopes
	case AuthAnyName:
		return checkAnyScopes
	}
	return checkUnknown
}

func (s *spec) Name() string {
	switch s.typ {
	case checkAnyScopes:
		return AuthAnyName
	case checkAllScopes:
		return AuthAllName
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

	f := &filter{typ: s.typ, authClient: ac}
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
