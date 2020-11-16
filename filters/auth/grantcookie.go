package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

type cookie struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry,omitempty"`
	RefreshAfter time.Time `json:"refresh_after,omitempty"`
}

func refreshAfter(expiry time.Time) time.Time {
	now := time.Now()
	d := expiry.Sub(now)
	if d <= 0 {
		return now
	}

	d /= 10
	if d < time.Minute {
		d = time.Minute
	}

	return now.Add(d)
}

func decodeCookie(cookieHeader string, config OAuthConfig) (c *cookie, err error) {
	var eb []byte
	if eb, err = base64.StdEncoding.DecodeString(cookieHeader); err != nil {
		return
	}

	var encryption secrets.Encryption
	if encryption, err = config.Secrets.GetEncrypter(secretsRefreshInternal, config.SecretFile); err != nil {
		return
	}

	var b []byte
	if b, err = encryption.Decrypt(eb); err != nil {
		return
	}

	err = json.Unmarshal(b, &c)
	return
}

func (c cookie) isAccessTokenExpired() bool {
	now := time.Now()
	return now.After(c.Expiry)
}

// GetCookie extracts the OAuth Grant token cookie from a HTTP request.
// The function supports multiple cookies with the same name and returns
// the best match (the one that decodes properly).
// The client may send multiple cookies if a parent domain has set a
// cookie of the same name.
func getCookie(request *http.Request, config OAuthConfig) (c *cookie, err error) {
	for _, c := range request.Cookies() {
		if c.Name == config.TokenCookieName {
			cookie, _ := decodeCookie(c.Value, config)
			if cookie != nil {
				return cookie, nil
			}
		}
	}
	return nil, http.ErrNoCookie
}

// Drops the grant token cookie from the request
func dropCookie(request *http.Request, config OAuthConfig) {
	cookies := request.Header.Get("Cookie")
	cookies = config.TokenCookieRegexp.ReplaceAllString(cookies, "")
	request.Header.Set("Cookie", cookies)
}

func CreateCookie(config OAuthConfig, host string, t *oauth2.Token) (*http.Cookie, error) {
	c := cookie{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Expiry:       t.Expiry,
	}

	if !config.DisableRefresh {
		c.RefreshAfter = refreshAfter(t.Expiry)
	}

	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	encryption, err := config.Secrets.GetEncrypter(secretsRefreshInternal, config.SecretFile)
	if err != nil {
		return nil, err
	}

	eb, err := encryption.Encrypt(b)
	if err != nil {
		return nil, err
	}

	b64 := base64.StdEncoding.EncodeToString(eb)

	// The cookie expiry date must not be the same as the access token
	// expiry. Otherwise the browser deletes the cookie as soon as the
	// access token expires, but _before_ the refresh token has expired.
	// Since we don't know the actual refresh token expiry, set it to
	// 30 days as a good compromise.
	return &http.Cookie{
		Name:     config.TokenCookieName,
		Value:    b64,
		Path:     "/",
		Domain:   extractDomainFromHost(host),
		Expires:  t.Expiry.Add(time.Hour * 24 * 30),
		Secure:   true,
		HttpOnly: true,
	}, nil
}
