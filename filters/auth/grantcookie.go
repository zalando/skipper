package auth

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

type CookieEncoder interface {
	// Update creates a set of cookies that encodes the token and deletes previously existing cookies if necessary.
	// When token is nil it only returns cookies to delete.
	Update(request *http.Request, token *oauth2.Token) ([]*http.Cookie, error)

	// Read extracts the token from the request cookies.
	Read(request *http.Request) (*oauth2.Token, error)
}

// EncryptedCookieEncoder is a CookieEncoder that encrypts the token before storing it in a cookie.
type EncryptedCookieEncoder struct {
	Encryption       secrets.Encryption
	CookieName       string
	RemoveSubdomains int
	Insecure         bool
}

var _ CookieEncoder = &EncryptedCookieEncoder{}

func (ce *EncryptedCookieEncoder) Update(request *http.Request, token *oauth2.Token) ([]*http.Cookie, error) {
	if token != nil {
		c, err := ce.createCookie(request.Host, token)
		if err != nil {
			return nil, err
		}
		return []*http.Cookie{c}, nil
	} else {
		c := ce.createDeleteCookie(request.Host)
		return []*http.Cookie{c}, nil
	}
}

func (ce *EncryptedCookieEncoder) Read(request *http.Request) (*oauth2.Token, error) {
	c, err := ce.extractCookie(request)
	if err != nil {
		return nil, err
	}

	return &oauth2.Token{
		AccessToken:  c.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: c.RefreshToken,
		Expiry:       c.Expiry,
	}, nil
}

type cookie struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Domain       string    `json:"domain,omitempty"`
}

func (ce *EncryptedCookieEncoder) decodeCookie(cookieHeader string) (c *cookie, err error) {
	var eb []byte
	if eb, err = base64.StdEncoding.DecodeString(cookieHeader); err != nil {
		return
	}

	var b []byte
	if b, err = ce.Encryption.Decrypt(eb); err != nil {
		return
	}

	err = json.Unmarshal(b, &c)
	return
}

// allowedForHost checks if provided host matches cookie domain
// according to https://www.rfc-editor.org/rfc/rfc6265#section-5.1.3
func (c *cookie) allowedForHost(host string) bool {
	hostWithoutPort, _, err := net.SplitHostPort(host)
	if err != nil {
		hostWithoutPort = host
	}
	return strings.HasSuffix(hostWithoutPort, c.Domain)
}

// extractCookie removes and returns the OAuth Grant token cookie from a HTTP request.
// The function supports multiple cookies with the same name and returns
// the best match (the one that decodes properly).
// The client may send multiple cookies if a parent domain has set a
// cookie of the same name.
// The grant token cookie is extracted so it does not get exposed to untrusted downstream
// services.
func (ce *EncryptedCookieEncoder) extractCookie(request *http.Request) (*cookie, error) {
	cookies := request.Cookies()
	for i, c := range cookies {
		if c.Name != ce.CookieName {
			continue
		}

		decoded, err := ce.decodeCookie(c.Value)
		if err == nil && decoded.allowedForHost(request.Host) {
			request.Header.Del("Cookie")
			for j, c := range cookies {
				if j != i {
					request.AddCookie(c)
				}
			}
			return decoded, nil
		}
	}
	return nil, http.ErrNoCookie
}

// createDeleteCookie creates a cookie, which instructs the client to clear the grant
// token cookie when used with a Set-Cookie header.
func (ce *EncryptedCookieEncoder) createDeleteCookie(host string) *http.Cookie {
	return &http.Cookie{
		Name:     ce.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   extractDomainFromHost(host, ce.RemoveSubdomains),
		MaxAge:   -1,
		Secure:   !ce.Insecure,
		HttpOnly: true,
	}
}

func (ce *EncryptedCookieEncoder) createCookie(host string, t *oauth2.Token) (*http.Cookie, error) {
	domain := extractDomainFromHost(host, ce.RemoveSubdomains)
	c := &cookie{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Expiry:       t.Expiry,
		Domain:       domain,
	}

	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	eb, err := ce.Encryption.Encrypt(b)
	if err != nil {
		return nil, err
	}

	b64 := base64.StdEncoding.EncodeToString(eb)

	// The cookie expiry date must not be the same as the access token
	// expiry. Otherwise, the browser deletes the cookie as soon as the
	// access token expires, but _before_ the refresh token has expired.
	// Since we don't know the actual refresh token expiry, set it to
	// 30 days as a good compromise.
	return &http.Cookie{
		Name:     ce.CookieName,
		Value:    b64,
		Path:     "/",
		Domain:   domain,
		Expires:  t.Expiry.Add(time.Hour * 24 * 30),
		Secure:   !ce.Insecure,
		HttpOnly: true,
	}, nil
}
