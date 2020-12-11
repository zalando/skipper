package auth

import (
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

const (
	TestToken                = "foobarbaz"
	TestRefreshToken         = "refreshfoobarbaz"
	TestAccessTokenExpiresIn = time.Hour
)

func NewGrantCookieWithExpiration(config OAuthConfig, expiry time.Time) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  TestToken,
		RefreshToken: TestRefreshToken,
		Expiry:       expiry,
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithInvalidAccessToken(config OAuthConfig) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  "invalid",
		RefreshToken: TestRefreshToken,
		Expiry:       time.Now().Add(TestAccessTokenExpiresIn),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithInvalidRefreshToken(config OAuthConfig) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  TestToken,
		RefreshToken: "invalid",
		Expiry:       time.Now().Add(time.Duration(-1) * time.Minute),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}

func NewGrantCookieWithTokens(config OAuthConfig, refreshToken string, accessToken string) (*http.Cookie, error) {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(TestAccessTokenExpiresIn),
	}

	cookie, err := createCookie(config, "", token)
	return cookie, err
}
