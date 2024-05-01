package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const (
	// These need to match with the values defined
	// in auth_test so make sure to keep them
	// synced if you happen to change one of them.
	testRefreshToken         = "refreshfoobarbaz"
	testAccessTokenExpiresIn = time.Hour
)

func newGrantCookies(t *testing.T, config *OAuthConfig, host string, token oauth2.Token) []*http.Cookie {
	cookies, err := config.GrantCookieEncoder.Update(&http.Request{Host: host}, &token)
	require.NoError(t, err)
	return cookies
}

func NewGrantCookies(t *testing.T, config *OAuthConfig) []*http.Cookie {
	return newGrantCookies(t, config, "", oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	})
}

func NewGrantCookiesWithExpiration(t *testing.T, config *OAuthConfig, expiry time.Time) []*http.Cookie {
	return newGrantCookies(t, config, "", oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       expiry,
	})
}

func NewGrantCookiesWithInvalidAccessToken(t *testing.T, config *OAuthConfig) []*http.Cookie {
	return newGrantCookies(t, config, "", oauth2.Token{
		AccessToken:  "invalid",
		RefreshToken: testRefreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	})
}

func NewGrantCookiesWithInvalidRefreshToken(t *testing.T, config *OAuthConfig) []*http.Cookie {
	return newGrantCookies(t, config, "", oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: "invalid",
		Expiry:       time.Now().Add(time.Duration(-1) * time.Minute),
	})
}

func NewGrantCookiesWithTokens(t *testing.T, config *OAuthConfig, refreshToken string, accessToken string) []*http.Cookie {
	return newGrantCookies(t, config, "", oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	})
}

func NewGrantCookiesWithHost(t *testing.T, config *OAuthConfig, host string) []*http.Cookie {
	return newGrantCookies(t, config, host, oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	})
}
