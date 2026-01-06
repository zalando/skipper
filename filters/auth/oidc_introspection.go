package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.OidcClaimsQueryName instead
	OidcClaimsQueryName = filters.OidcClaimsQueryName

	oidcClaimsCacheKey = "oidcclaimscachekey"
)

var gjsonMu sync.RWMutex

type (
	oidcIntrospectionSpec struct {
		typ roleCheckType
	}
	oidcIntrospectionFilter struct {
		typ   roleCheckType
		paths []pathQuery
	}

	pathQuery struct {
		path    string
		queries []string
	}
)

func NewOIDCQueryClaimsFilter() filters.Spec {
	return &oidcIntrospectionSpec{
		typ: checkOIDCQueryClaims,
	}
}

// SetOIDCClaims sets OIDC claims in the state bag.
// Intended for use with the oidcClaimsQuery filter.
func SetOIDCClaims(ctx filters.FilterContext, claims map[string]interface{}) {
	ctx.StateBag()[oidcClaimsCacheKey] = tokenContainer{
		Claims: claims,
	}
}

func (spec *oidcIntrospectionSpec) Name() string {
	switch spec.typ {
	case checkOIDCQueryClaims:
		return filters.OidcClaimsQueryName
	}
	return AuthUnknown
}

func (spec *oidcIntrospectionSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	filter := &oidcIntrospectionFilter{typ: spec.typ}

	switch filter.typ {
	case checkOIDCQueryClaims:
		for _, arg := range sargs {
			path, queries, found := strings.Cut(arg, ":")
			if !found || path == "" {
				return nil, fmt.Errorf("%v: malformed filter arg %s", filters.ErrInvalidFilterParameters, arg)
			}
			pq := pathQuery{path: path}
			for _, query := range splitQueries(queries) {
				if query == "" {
					return nil, fmt.Errorf("%w: %s", errUnsupportedClaimSpecified, arg)
				}
				pq.queries = append(pq.queries, trimQuotes(query))
			}
			if len(pq.queries) == 0 {
				return nil, fmt.Errorf("%w: %s", errUnsupportedClaimSpecified, arg)
			}
			filter.paths = append(filter.paths, pq)
		}
		if len(filter.paths) == 0 {
			return nil, fmt.Errorf("%w: no queries could be parsed", errUnsupportedClaimSpecified)
		}
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	gjsonMu.RLock()
	// method is not thread safe
	modExists := gjson.ModifierExists("_", gjsonThisModifier)
	gjsonMu.RUnlock()

	if !modExists {
		gjsonMu.Lock()
		// method is not thread safe
		gjson.AddModifier("_", gjsonThisModifier)
		gjsonMu.Unlock()
	}

	return filter, nil
}

func (filter *oidcIntrospectionFilter) String() string {
	var str []string
	for _, query := range filter.paths {
		str = append(str, query.String())
	}
	return fmt.Sprintf("%s(%s)", filters.OidcClaimsQueryName, strings.Join(str, "; "))
}

func (filter *oidcIntrospectionFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	token, ok := ctx.StateBag()[oidcClaimsCacheKey].(tokenContainer)
	if !ok || &token == (&tokenContainer{}) || len(token.Claims) == 0 {
		ctx.Logger().Errorf("Error retrieving %s for OIDC token introspection", oidcClaimsCacheKey)
		unauthorized(ctx, "", missingToken, r.Host, oidcClaimsCacheKey+" is unavailable in StateBag")
		return
	}

	switch filter.typ {
	case checkOIDCQueryClaims:
		if !filter.validateClaimsQuery(r.URL.Path, token.Claims) {
			unauthorized(ctx, "", invalidAccess, r.Host, "Path not permitted")
			return
		}
	default:
		unauthorized(ctx, fmt.Sprint(filter.typ), invalidClaim, r.Host, "Wrong oidcIntrospectionFilter type")
		return
	}

	sub, ok := token.Claims["sub"].(string)
	if !ok {
		unauthorized(ctx, fmt.Sprint(filter.typ), invalidSub, r.Host, "Invalid Subject")
		return
	}

	authorized(ctx, sub)
}

func (filter *oidcIntrospectionFilter) Response(filters.FilterContext) {}

func gjsonThisModifier(json, arg string) string {
	return gjson.Get(json, "[@this].#("+arg+")").Raw
}

func (filter *oidcIntrospectionFilter) validateClaimsQuery(reqPath string, gotToken map[string]interface{}) bool {
	l := len(filter.paths)
	if l == 0 {
		return false
	}

	json, err := json.Marshal(gotToken)
	if err != nil || !gjson.ValidBytes(json) {
		log.Errorf("Failed to serialize in validateClaimsQuery: %v", err)
		return false
	}
	parsed := gjson.ParseBytes(json)

	for _, path := range filter.paths {
		if !strings.HasPrefix(reqPath, path.path) {
			continue
		}

		for _, query := range path.queries {
			match := parsed.Get(query)
			log.Debugf("claim: %s results:%s", query, match.String())
			if match.Exists() {
				return true
			}
		}
		return false
	}
	return false
}

func (p pathQuery) String() string {
	return fmt.Sprintf("path: '%s*', matching: %s", p.path, strings.Join(p.queries, " ,"))
}

// Splits space-delimited GJSON queries ignoring spaces within quoted strings
func splitQueries(s string) (q []string) {
	for _, p := range strings.Split(s, " ") {
		if len(q) == 0 || strings.Count(q[len(q)-1], `"`)%2 == 0 {
			q = append(q, p)
		} else {
			q[len(q)-1] = q[len(q)-1] + " " + p
		}
	}
	return
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if c := s[len(s)-1]; s[0] == c && (c == '"' || c == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
