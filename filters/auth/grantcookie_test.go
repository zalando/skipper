package auth

import (
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

const (
	// These need to match with the values defined
	// in auth_test so make sure to keep them
	// synced if you happen to change one of them.
	testRefreshToken         = "refreshfoobarbaz"
	testAccessTokenExpiresIn = time.Hour
)

func NewGrantCookieWithExpiration(config *OAuthConfig, expiry time.Time) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       expiry,
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithInvalidAccessToken(config *OAuthConfig) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  "invalid",
		RefreshToken: testRefreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithInvalidRefreshToken(config *OAuthConfig) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: "invalid",
		Expiry:       time.Now().Add(time.Duration(-1) * time.Minute),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithTokens(config *OAuthConfig, refreshToken string, accessToken string) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithHost(config *OAuthConfig, host string) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  testToken,
		RefreshToken: testRefreshToken,
		Expiry:       time.Now().Add(testAccessTokenExpiresIn),
	}

	return createCookie(config, host, token)
}
