package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

const (
	OAuthGrantName       = "oauthGrant"
	OAuthGrantCookieName = "oauth-token"

	bearerPrefix           = "Bearer "
	secretsRefreshInternal = time.Minute
	oauthGrantTokenKey = "oauth-grant-token"
)

type OAuthConfig struct {

	// TokeninfoURL is the URL of the service to validate OAuth2 tokens.
	TokeninfoURL string

	// ProvideLogin, when true enables OAuth2 Grants Flow login on requests
	// that don't have either a valid Bearer token or an access cookie. It
	// only provides login for requests that indicate a browser via their
	// Accept header including text/html.
	ProvideLogin bool

	// Secrets is a secret registry to access secret keys used for encrypting
	// auth flow state and auth cookies. It's mandatory when ProvideLogin is
	// true.
	Secrets *secrets.Registry

	// SecretsFile contains the encryption key for the authentication cookie
	// and grant flow state.
	SecretFile string

	// AuthURL, the url to redirect the requests to when login is require.
	AuthURL string

	// TokenURL, the url where the access code should be exchanged for the
	// access token.
	TokenURL string

	// CallbackPath, path used for the authentication callbacks, when
	// redirected from AuthURL.
	CallbackPath string

	// ClientID, the id of the current service, used to exchange the access
	// code.
	ClientID string

	// ClientSecret, the secret associated with the ClientID, used to exchange
	// the access code.
	ClientSecret string

	// TokeninfoClient, optional. When set, it will be used for the
	// authorization requests to TokeninfoURL. When not set, a new default
	// client is created.
	TokeninfoClient *http.Client

	// AuthClient, optional. When set, it will be used for the
	// access code exchange requests to TokenURL. When not set, a new default
	// client is created.
	AuthClient *http.Client

	// DisableRefresh prevents refreshing the token.
	DisableRefresh bool
}

type cookie struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry,omitempty"`
	RefreshAfter time.Time `json:"refresh_after,omitempty"`
}

type spec struct{}

type filter struct {
	config      OAuthConfig
	oauthConfig *oauth2.Config
	flowState   *flowState
}

func NewGrant(OAuthConfig) (filters.Spec, error) {
	return &spec{}, nil
}

func (s *spec) Name() string { return OAuthGrantName }

func (s *spec) CreateFilter([]interface{}) (filters.Filter, error) {
	return &filter{}, nil
}

func (f *filter) validateToken(t string) (bool, error) {
	if !strings.HasPrefix(t, bearerPrefix) || len(t) == len(bearerPrefix) {
		return false, nil
	}

	req, err := http.NewRequest("GET", f.config.TokeninfoURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating request to tokeninfo failed: %w", err)
	}
	req.Header.Set("Authorization", t)

	resp, err := f.config.TokeninfoClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("request to tokeninfo failed: %w", err)
	}
	defer resp.Body.Close()

	// TODO: perform actual validation of scopes
	return resp.StatusCode == 200, nil
}

func (f *filter) refreshToken(c cookie) (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	}

	ctx := f.providerContext()

	// oauth2.TokenSource implements the refresh functionality,
	// we're hijacking it here.
	tokenSource := f.oauthConfig.TokenSource(ctx, token)
	return tokenSource.Token()
}

func (f *filter) providerContext() context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, f.config.AuthClient)
}

func (f *filter) loginRedirect(ctx filters.FilterContext) {
	req := ctx.Request()

	state, err := f.flowState.createState(req.URL.String())
	if err != nil {
		log.Errorf("failed to create login redirect: %v", err)
		return
	}

	authConfig := *f.oauthConfig
	authConfig.RedirectURL = req.URL.String()
	ctx.Serve(&http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": []string{authConfig.AuthCodeURL(state)},
		},
	})
}

func (f *filter) decodeCookie(s string) (c cookie, err error) {
	var eb []byte
	if eb, err = base64.StdEncoding.DecodeString(s); err != nil {
		return
	}

	var encryption secrets.Encryption
	if encryption, err = f.config.Secrets.GetEncrypter(secretsRefreshInternal, f.config.SecretFile); err != nil {
		return
	}

	var b []byte
	if b, err = encryption.Decrypt(eb); err != nil {
		return
	}

	err = json.Unmarshal(b, &c)
	return
}

func (f *filter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	c, err := req.Cookie(OAuthGrantCookieName)
	if err == http.ErrNoCookie {
		f.loginRedirect(ctx)
		return
	}

	cc, err := f.decodeCookie(c.Value)
	if err != nil {
		log.Debugf("Error while decoding cookie: %v", err)
		f.loginRedirect(ctx)
		return
	}

	now := time.Now()

	var valid bool
	if cc.Expiry.After(now) {
		var err error
		if valid, err = f.validateToken(bearerPrefix + cc.AccessToken); err != nil {
			log.Errorf("Error while validating bearer token: %v.", err)
			ctx.Serve(&http.Response{StatusCode: http.StatusInternalServerError})
			return
		}
	}

	canRefresh := !f.config.DisableRefresh && cc.RefreshToken != ""
	shouldRefresh := !valid || cc.RefreshAfter.Before(now)
	if canRefresh && shouldRefresh {
		token, err := f.refreshToken(cc)
		if err != nil {
			log.Debugf("Error while refreshing token: %v.", err)
			if !valid {
				f.loginRedirect(ctx)
				return
			}
		}

		// we set the refreshed cookie once we have a response
		ctx.StateBag()[oauthGrantTokenKey] = token
		return
	}

	if !valid {
		f.loginRedirect(ctx)
	}
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

func (f *filter) createCookie(host string, t *oauth2.Token) (*http.Cookie, error) {
	c := cookie{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Expiry:       t.Expiry,
	}

	if !f.config.DisableRefresh {
		c.RefreshAfter = refreshAfter(t.Expiry)
	}

	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	encryption, err := f.config.Secrets.GetEncrypter(secretsRefreshInternal, f.config.SecretFile)
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
		SameSite: http.SameSiteStrictMode,
	}, nil
}

func (f *filter) Response(ctx filters.FilterContext) {
	token, ok := ctx.StateBag()[oauthGrantTokenKey].(*oauth2.Token)
	if !ok {
		return
	}

	req := ctx.Request()
	c, err := f.createCookie(req.URL.Host, token)
	if err != nil {
		log.Errorf("Error while generating cookie: %v.", err)
		return
	}

	rsp := ctx.Response()
	rsp.Header.Add("Set-Cookie", c.String())
}
