package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

const OAuthGrantCookieName = "oauth-token"

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

func createCookie(config OAuthConfig, host string, t *oauth2.Token) (*http.Cookie, error) {
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
	return &http.Cookie{
		Name:     OAuthGrantCookieName,
		Value:    b64,
		Path:     "/",
		Domain:   extractDomainFromHost(host),
		Expires:  t.Expiry,
		Secure:   true,
		HttpOnly: true,
	}, nil
}
