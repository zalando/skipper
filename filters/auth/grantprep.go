package auth

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
)

const (
	defaultCallbackRouteID = "__oauth2_grant_callback"
	defaultCallbackPath    = "/.well-known/oauth2-callback"
	defaultTokenCookieName = "oauth-grant"
)

type grantPrep struct {
	config *OAuthConfig
}

func (p *grantPrep) Do(r []*eskip.Route) []*eskip.Route {
	// In the future, route IDs will serve only logging purpose and won't
	// need to be unique.
	id := defaultCallbackRouteID

	return append(r, &eskip.Route{
		Id: id,
		Predicates: []*eskip.Predicate{{
			Name: "Path",
			Args: []any{
				p.config.CallbackPath,
			},
		}},
		Filters: []*eskip.Filter{{
			Name: filters.GrantCallbackName,
		}},
		BackendType: eskip.ShuntBackend,
	})
}
