package auth

import (
	"errors"
	"fmt"
	"net/http"
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
	checkOAuthTokenintrospectionAnyClaims
	checkOAuthTokenintrospectionAllClaims
	checkOAuthTokenintrospectionAnyKV
	checkOAuthTokenintrospectionAllKV
	checkSecureOAuthTokenintrospectionAnyClaims
	checkSecureOAuthTokenintrospectionAllClaims
	checkSecureOAuthTokenintrospectionAnyKV
	checkSecureOAuthTokenintrospectionAllKV
	checkOIDCUserInfo
	checkOIDCAnyClaims
	checkOIDCAllClaims
)

type rejectReason string

const (
	missingBearerToken rejectReason = "missing-bearer-token"
	missingToken       rejectReason = "missing-token"
	authServiceAccess  rejectReason = "auth-service-access"
	invalidSub         rejectReason = "invalid-sub-in-token"
	inactiveToken      rejectReason = "inactive-token"
	invalidToken       rejectReason = "invalid-token"
	invalidScope       rejectReason = "invalid-scope"
	invalidClaim       rejectReason = "invalid-claim"
	invalidFilter      rejectReason = "invalid-filter"
	invalidAccess      rejectReason = "invalid-access"
)

const (
	AuthUnknown = "authUnknown"

	authHeaderName   = "Authorization"
	authHeaderPrefix = "Bearer "
	// tokenKey defined at https://tools.ietf.org/html/rfc7662#section-2.1
	tokenKey = "token"
	scopeKey = "scope"
	uidKey   = "uid"
)

type kv map[string][]string

type requestError struct {
	err error
}

var (
	errUnsupportedClaimSpecified     = errors.New("unsupported claim specified in filter")
	errInvalidToken                  = errors.New("invalid token")
	errInvalidTokenintrospectionData = errors.New("invalid tokenintrospection data")
)

func (kv kv) String() string {
	var res []string
	for k, v := range kv {
		res = append(res, k, strings.Join(v, " "))
	}
	return strings.Join(res, ",")
}

func (err *requestError) Error() string {
	return err.err.Error()
}

func requestErrorf(f string, args ...interface{}) error {
	return &requestError{
		err: fmt.Errorf(f, args...),
	}
}

func getToken(r *http.Request) (string, bool) {
	h := r.Header.Get(authHeaderName)
	if !strings.HasPrefix(h, authHeaderPrefix) {
		return "", false
	}

	return h[len(authHeaderPrefix):], true
}

func reject(
	ctx filters.FilterContext,
	status int,
	username string,
	reason rejectReason,
	hostname,
	debuginfo string,
) {
	if debuginfo == "" {
		log.Debugf(
			"Rejected: status: %d, username: %s, reason: %s.",
			status, username, reason,
		)
	} else {
		log.Debugf(
			"Rejected: status: %d, username: %s, reason: %s, info: %s.",
			status, username, reason, debuginfo,
		)
	}

	ctx.StateBag()[logfilter.AuthUserKey] = username
	ctx.StateBag()[logfilter.AuthRejectReasonKey] = string(reason)
	rsp := &http.Response{
		StatusCode: status,
		Header:     make(map[string][]string),
	}

	if hostname != "" {
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.4.2
		rsp.Header.Add("WWW-Authenticate", hostname)
	}

	ctx.Serve(rsp)
}

func unauthorized(ctx filters.FilterContext, username string, reason rejectReason, hostname, debuginfo string) {
	reject(ctx, http.StatusUnauthorized, username, reason, hostname, debuginfo)
}

func forbidden(ctx filters.FilterContext, username string, reason rejectReason, debuginfo string) {
	reject(ctx, http.StatusForbidden, username, reason, "", debuginfo)
}

func authorized(ctx filters.FilterContext, username string) {
	ctx.StateBag()[logfilter.AuthUserKey] = username
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
