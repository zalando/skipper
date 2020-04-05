package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

const (
	OAuthGrantName = "oauthGrant"

	bearerPrefix              = "Bearer "
	secretsRefreshInternal    = time.Minute
	oauthGrantRefreshTokenKey = "oauth-grant-token"
)

type cookie struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry,omitempty"`
	RefreshAfter time.Time `json:"refresh_after,omitempty"`
}

type grantSpec struct {
	config OAuthConfig
}

type grantFilter struct {
	config OAuthConfig
}

func (s grantSpec) Name() string { return OAuthGrantName }

func (s grantSpec) CreateFilter([]interface{}) (filters.Filter, error) {
	return grantFilter(s), nil
}

func providerContext(c OAuthConfig) context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, c.AuthClient)
}

func serverError(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusInternalServerError,
	})
}

func badRequest(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusBadRequest,
	})
}

func (f grantFilter) redirectURLs(req *http.Request) (redirect, original string) {
	u := *req.URL

	if fp := req.Header.Get("X-Forwarded-Proto"); fp != "" {
		u.Scheme = fp
	} else if req.TLS != nil {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	if fh := req.Header.Get("X-Forwarded-Host"); fh != "" {
		u.Host = fh
	} else {
		u.Host = req.Host
	}

	original = u.String()

	u.Path = f.config.CallbackPath
	u.RawQuery = ""
	redirect = u.String()
	return
}

func (f grantFilter) loginRedirect(ctx filters.FilterContext) {
	req := ctx.Request()
	redirect, original := f.redirectURLs(req)

	state, err := f.config.flowState.createState(original)
	if err != nil {
		log.Errorf("failed to create login redirect: %v", err)
		serverError(ctx)
		return
	}

	authConfig := *f.config.oauthConfig
	authConfig.RedirectURL = redirect
	ctx.Serve(&http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": []string{authConfig.AuthCodeURL(state)},
		},
	})
}

func (f grantFilter) decodeCookie(s string) (c cookie, err error) {
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

func (f grantFilter) validateToken(t string) (bool, error) {
	req, err := http.NewRequest("GET", f.config.TokeninfoURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating request to tokeninfo failed: %w", err)
	}
	req.Header.Set("Authorization", bearerPrefix+t)

	rsp, err := f.config.TokeninfoClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("request to tokeninfo failed: %w", err)
	}
	defer rsp.Body.Close()

	// TODO: scope validation
	return rsp.StatusCode == 200, nil
}

func (f grantFilter) refreshToken(c cookie) (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	}

	ctx := providerContext(f.config)

	// oauth2.TokenSource implements the refresh functionality,
	// we're hijacking it here.
	tokenSource := f.config.oauthConfig.TokenSource(ctx, token)
	return tokenSource.Token()
}

func (f grantFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	c, err := req.Cookie(OAuthGrantCookieName)
	if err == http.ErrNoCookie {
		f.loginRedirect(ctx)
		return
	}

	cc, err := f.decodeCookie(c.Value)
	if err != nil {
		log.Debugf("Error while decoding cookie: %v.", err)
		f.loginRedirect(ctx)
		return
	}

	now := time.Now()

	var valid bool
	if now.Before(cc.Expiry) {
		var err error
		if valid, err = f.validateToken(cc.AccessToken); err != nil {
			log.Errorf("Error while validating bearer token: %v.", err)
			serverError(ctx)
			return
		}
	}

	canRefresh := !f.config.DisableRefresh && cc.RefreshToken != ""
	shouldRefresh := !valid || now.After(cc.RefreshAfter)
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
		ctx.StateBag()[oauthGrantRefreshTokenKey] = token
		return
	}

	if !valid {
		f.loginRedirect(ctx)
	}
}

func (f grantFilter) Response(ctx filters.FilterContext) {
	token, ok := ctx.StateBag()[oauthGrantRefreshTokenKey].(*oauth2.Token)
	if !ok {
		return
	}

	req := ctx.Request()
	c, err := createCookie(f.config, req.Host, token)
	if err != nil {
		log.Errorf("Error while generating cookie: %v.", err)
		return
	}

	rsp := ctx.Response()
	rsp.Header.Add("Set-Cookie", c.String())
}
