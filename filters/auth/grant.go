package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

const (
	OAuthGrantName = "oauthGrant"

	secretsRefreshInternal    = time.Minute
	oauthGrantRefreshTokenKey = "oauth-grant-token"
)

var (
	errExpiredToken = errors.New("expired access token")
)

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

func loginRedirect(ctx filters.FilterContext, config OAuthConfig) {
	loginRedirectWithOverride(ctx, config, "")
}

func loginRedirectWithOverride(ctx filters.FilterContext, config OAuthConfig, originalOverride string) {
	req := ctx.Request()
	redirect, original := config.RedirectURLs(req)

	if originalOverride != "" {
		original = originalOverride
	}

	state, err := config.flowState.createState(original)
	if err != nil {
		log.Errorf("Failed to create login redirect: %v", err)
		serverError(ctx)
		return
	}

	authConfig := config.GetConfig()
	ctx.Serve(&http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": []string{authConfig.AuthCodeURL(state, config.GetAuthURLParameters(redirect)...)},
		},
	})
}

func (f grantFilter) refreshToken(c cookie) (*oauth2.Token, error) {
	// Set the expiry of the token to the past to trigger oauth2.TokenSource
	// to refresh the access token.
	token := &oauth2.Token{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	}

	ctx := providerContext(f.config)

	// oauth2.TokenSource implements the refresh functionality,
	// we're hijacking it here.
	tokenSource := f.config.GetConfig().TokenSource(ctx, token)
	return tokenSource.Token()
}

func (f grantFilter) refreshTokenIfRequired(c cookie) (*oauth2.Token, error) {
	now := time.Now()
	isAccessTokenExpired := c.isAccessTokenExpired()
	canRefresh := !f.config.DisableRefresh && c.RefreshToken != ""
	shouldRefresh := isAccessTokenExpired || now.After(c.RefreshAfter)

	if shouldRefresh {
		if canRefresh {
			return f.refreshToken(c)
		} else {
			return nil, errExpiredToken
		}
	} else {
		return &oauth2.Token{
			AccessToken:  c.AccessToken,
			TokenType:    "Bearer",
			RefreshToken: c.RefreshToken,
			Expiry:       c.Expiry,
		}, nil
	}
}

func (f grantFilter) setAccessTokenHeader(req *http.Request, token string) {
	if f.config.AccessTokenHeaderName != "" {
		req.Header.Set(f.config.AccessTokenHeaderName, authHeaderPrefix+token)
	}
}

func (f grantFilter) createTokenContainer(token *oauth2.Token, tokeninfo map[string]interface{}) tokenContainer {
	subject := ""
	if f.config.TokeninfoSubjectKey != "" {
		subject = tokeninfo[f.config.TokeninfoSubjectKey].(string)
	}

	tokeninfo["sub"] = subject

	return tokenContainer{
		OAuth2Token: token,
		Subject:     subject,
		Claims:      tokeninfo,
	}
}

func (f grantFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	c, err := extractCookie(req, f.config)
	if err == http.ErrNoCookie {
		loginRedirect(ctx, f.config)
		return
	}

	token, err := f.refreshTokenIfRequired(*c)
	if err != nil && c.isAccessTokenExpired() {
		// Refresh failed and we no longer have a valid access token.
		loginRedirect(ctx, f.config)
		return
	}

	tokeninfo, err := f.config.TokeninfoClient.getTokeninfo(token.AccessToken, ctx)
	if err != nil || tokeninfo == nil {
		if err == errInvalidToken {
			log.Errorf("Error while calling tokeninfo: %v.", err)
		}
		loginRedirect(ctx, f.config)
		return
	}

	f.setAccessTokenHeader(req, token.AccessToken)

	// Set token in state bag for response Set-Cookie. By piggy-backing
	// on the OIDC token container, we gain downstream compatibility with
	// the oidcClaimsQuery filter.
	ctx.StateBag()[oidcClaimsCacheKey] = f.createTokenContainer(token, tokeninfo)

	// Set the tokeninfo also in the tokeninfoCacheKey state bag, so we
	// can reuse e.g. the forwardToken() filter.
	ctx.StateBag()[tokeninfoCacheKey] = tokeninfo
}

func (f grantFilter) Response(ctx filters.FilterContext) {
	container, ok := ctx.StateBag()[oidcClaimsCacheKey].(tokenContainer)
	if !ok {
		return
	}

	req := ctx.Request()
	c, err := CreateCookie(f.config, req.Host, container.OAuth2Token)
	if err != nil {
		log.Errorf("Error while generating cookie: %v.", err)
		return
	}

	rsp := ctx.Response()
	rsp.Header.Add("Set-Cookie", c.String())
}
