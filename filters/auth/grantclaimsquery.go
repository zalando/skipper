//
// grantClaimsQuery filter
//
// An alias for oidcClaimsQuery filter allowing a clearer
// API when used in conjunction with the oauthGrant filter.
//

package auth

import "github.com/zalando/skipper/filters"

type grantClaimsQuerySpec struct {
	oidcSpec oidcIntrospectionSpec
}

func (s *grantClaimsQuerySpec) Name() string {
	return filters.GrantClaimsQueryName
}

func (s *grantClaimsQuerySpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	return s.oidcSpec.CreateFilter(args)
}
