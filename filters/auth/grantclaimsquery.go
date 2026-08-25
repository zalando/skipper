//
// grantClaimsQuery filter
//
// An alias for oidcClaimsQuery filter allowing a clearer
// API when used in conjunction with the oauthGrant filter.
//

package auth

import "github.com/zalando/skipper/filters"

// GrantClaimsQueryName is the filter name
// Deprecated, use filters.GrantClaimsQueryName instead
const GrantClaimsQueryName = filters.GrantClaimsQueryName

type grantClaimsQuerySpec struct {
	oidcSpec oidcIntrospectionSpec
}

func (s *grantClaimsQuerySpec) Name() string {
	return filters.GrantClaimsQueryName
}

func (s *grantClaimsQuerySpec) CreateFilter(args []any) (filters.Filter, error) {
	return s.oidcSpec.CreateFilter(args)
}
