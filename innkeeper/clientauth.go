package innkeeper

import (
	"github.com/zalando/skipper/oauth"
)

// An Authentication object provides authentication to Innkeeper.
type Authentication interface {
	GetToken() (string, error)
}

type AuthOptions struct {
	InnkeeperAuthToken  string
	OAuthCredentialsDir string
	OAuthUrl            string
	OAuthScope          string
}

// A FixedToken provides Innkeeper authentication by an unchanged token
// string.
type FixedToken string

// Returns the fixed token.
func (ft FixedToken) GetToken() (string, error) { return string(ft), nil }

func CreateInnkeeperAuthentication(o AuthOptions) Authentication {
	if o.InnkeeperAuthToken != "" {
		return FixedToken(o.InnkeeperAuthToken)
	}

	return oauth.New(o.OAuthCredentialsDir, o.OAuthUrl, o.OAuthScope)
}
