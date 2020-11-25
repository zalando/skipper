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

// extractCookie removes and returns the OAuth Grant token cookie from a HTTP request.
// The function supports multiple cookies with the same name and returns
// the best match (the one that decodes properly).
// The client may send multiple cookies if a parent domain has set a
// cookie of the same name.
// The grant token cookie is extracted so it does not get exposed to untrusted downstream
// services.
func extractCookie(request *http.Request, config OAuthConfig) (cookie *cookie, err error) {
	old := request.Cookies()
	new := make([]*http.Cookie, 0, len(old))

	for i, c := range old {
		if c.Name == config.TokenCookieName {
			cookie, _ = decodeCookie(c.Value, config)
			if cookie != nil {
				new = append(new, old[i+1:]...)
				break
			}
		}
		new = append(new, c)
	}

	if cookie != nil {
		request.Header.Del("Cookie")
		for _, c := range new {
			request.AddCookie(c)
		}
		return cookie, nil
	}
	return nil, http.ErrNoCookie
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
