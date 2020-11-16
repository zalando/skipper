//
// grantClaimsQuery filter
//
// An alias for oidcClaimsQuery filter allowing a clearer
// API when used in conjunction with the oauthGrant filter.
//

package auth

import "github.com/zalando/skipper/filters"

const GrantClaimsQueryName = "grantClaimsQuery"

type grantClaimsQuerySpec struct {
	oidcSpec oidcIntrospectionSpec
}

func (s grantClaimsQuerySpec) Name() string {
	return GrantClaimsQueryName
}

func (s grantClaimsQuerySpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	return s.oidcSpec.CreateFilter(args)
}
