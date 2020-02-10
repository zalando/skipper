package auth

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	OidcClaimsQueryName = "oidcClaimsQuery"
	oidcClaimsCacheKey  = "oidcclaimscachekey"
)

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

func (spec *oidcIntrospectionSpec) Name() string {
	switch spec.typ {
	case checkOIDCQueryClaims:
		return OidcClaimsQueryName
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
			slice := strings.SplitN(arg, ":", 2)
			if len(slice) != 2 {
				return nil, fmt.Errorf("%v: malformatted filter arg %s", filters.ErrInvalidFilterParameters, arg)
			}
			pq := pathQuery{path: slice[0]}
			for _, query := range strings.Split(slice[1], " ") {
				if query == "" {
					return nil, fmt.Errorf("%v: %s", errUnsupportedClaimSpecified, arg)
				}
				pq.queries = append(pq.queries, trimQuotes(query))
			}
			if len(pq.queries) == 0 {
				return nil, fmt.Errorf("%v: %s", errUnsupportedClaimSpecified, arg)
			}
			filter.paths = append(filter.paths, pq)
		}
		if len(filter.paths) == 0 {
			return nil, fmt.Errorf("%v: no queries could be parsed", errUnsupportedClaimSpecified)
		}
	default:
		return nil, filters.ErrInvalidFilterParameters
	}

	return filter, nil
}

func (filter *oidcIntrospectionFilter) String() string {
	var str []string
	for _, query := range filter.paths {
		str = append(str, query.String())
	}
	return fmt.Sprintf("%s(%s)", OidcClaimsQueryName, strings.Join(str, "; "))
}

func (filter *oidcIntrospectionFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	token, ok := ctx.StateBag()[oidcClaimsCacheKey].(tokenContainer)
	if !ok || &token == (&tokenContainer{}) || len(token.Claims) == 0 {
		log.Errorf("Error retrieving %s for OIDC token introspection", oidcClaimsCacheKey)
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
		unauthorized(ctx, string(filter.typ), invalidClaim, r.Host, "Wrong oidcIntrospectionFilter type")
		return
	}

	sub := token.Claims["sub"].(string)
	authorized(ctx, sub)
}

func (filter *oidcIntrospectionFilter) Response(filters.FilterContext) {}

func (filter *oidcIntrospectionFilter) validateClaimsQuery(reqPath string, gotToken map[string]interface{}) bool {
	gjson.AddModifier("_", func(json, arg string) string {
		return gjson.Get(json, "[@this].#("+arg+")").Raw
	})

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

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if c := s[len(s)-1]; s[0] == c && (c == '"' || c == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
