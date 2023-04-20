package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

const (
	// Deprecated, use filters.OAuthGrantName instead
	OAuthGrantName = filters.OAuthGrantName

	secretsRefreshInternal = time.Minute
	refreshedTokenKey      = "oauth-refreshed-token"
)

var (
	errExpiredToken = errors.New("expired access token")
)

type grantSpec struct {
	config *OAuthConfig
}

type grantFilter struct {
	config *OAuthConfig
}

func (s *grantSpec) Name() string { return filters.OAuthGrantName }

func (s *grantSpec) CreateFilter([]interface{}) (filters.Filter, error) {
	return &grantFilter{
		config: s.config,
	}, nil
}

func providerContext(c *OAuthConfig) context.Context {
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

func loginRedirect(ctx filters.FilterContext, config *OAuthConfig) {
	loginRedirectWithOverride(ctx, config, "")
}

func loginRedirectWithOverride(ctx filters.FilterContext, config *OAuthConfig, originalOverride string) {
	req := ctx.Request()

	authConfig, err := config.GetConfig(req)
	if err != nil {
		ctx.Logger().Debugf("Failed to obtain auth config: %v", err)
		ctx.Serve(&http.Response{
			StatusCode: http.StatusForbidden,
		})
		return
	}

	redirect, original := config.RedirectURLs(req)

	if originalOverride != "" {
		original = originalOverride
	}

	state, err := config.flowState.createState(original)
	if err != nil {
		ctx.Logger().Errorf("Failed to create login redirect: %v", err)
		serverError(ctx)
		return
	}

	ctx.Serve(&http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": []string{authConfig.AuthCodeURL(state, config.GetAuthURLParameters(redirect)...)},
		},
	})
}

func (f *grantFilter) refreshToken(c *cookie, req *http.Request) (*oauth2.Token, error) {
	// Set the expiry of the token to the past to trigger oauth2.TokenSource
	// to refresh the access token.
	token := &oauth2.Token{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	}

	ctx := providerContext(f.config)

	authConfig, err := f.config.GetConfig(req)
	if err != nil {
		return nil, err
	}

	// oauth2.TokenSource implements the refresh functionality,
	// we're hijacking it here.
	tokenSource := authConfig.TokenSource(ctx, token)
	return tokenSource.Token()
}

func (f *grantFilter) refreshTokenIfRequired(c *cookie, ctx filters.FilterContext) (*oauth2.Token, error) {
	canRefresh := c.RefreshToken != ""

	if c.isAccessTokenExpired() {
		if canRefresh {
			token, err := f.refreshToken(c, ctx.Request())
			if err == nil {
				// Remember that this token was just successfully refreshed
				// so that we can send an updated cookie in the response.
				ctx.StateBag()[refreshedTokenKey] = token
			}
			return token, err
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

func (f *grantFilter) setupToken(token *oauth2.Token, tokeninfo map[string]interface{}, ctx filters.FilterContext) error {
	if f.config.AccessTokenHeaderName != "" {
		ctx.Request().Header.Set(f.config.AccessTokenHeaderName, authHeaderPrefix+token.AccessToken)
	}

	subject := ""
	if f.config.TokeninfoSubjectKey != "" {
		if s, ok := tokeninfo[f.config.TokeninfoSubjectKey].(string); ok {
			subject = s
		} else {
			return fmt.Errorf("tokeninfo subject key '%s' is missing", f.config.TokeninfoSubjectKey)
		}
	}

	tokeninfo["sub"] = subject

	if len(f.config.grantTokeninfoKeysLookup) > 0 {
		for key := range tokeninfo {
			if _, ok := f.config.grantTokeninfoKeysLookup[key]; !ok {
				delete(tokeninfo, key)
			}
		}
	}

	// By piggy-backing on the OIDC token container,
	// we gain downstream compatibility with the oidcClaimsQuery filter.
	ctx.StateBag()[oidcClaimsCacheKey] = tokenContainer{
		OAuth2Token: token,
		Subject:     subject,
		Claims:      tokeninfo,
	}

	// Set the tokeninfo also in the tokeninfoCacheKey state bag, so we
	// can reuse e.g. the forwardToken() filter.
	ctx.StateBag()[tokeninfoCacheKey] = tokeninfo

	return nil
}

func (f *grantFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()

	c, err := extractCookie(req, f.config)
	if err == http.ErrNoCookie {
		loginRedirect(ctx, f.config)
		return
	}

	token, err := f.refreshTokenIfRequired(c, ctx)
	if err != nil {
		// Refresh failed and we no longer have a valid access token.
		loginRedirect(ctx, f.config)
		return
	}

	tokeninfo, err := f.config.TokeninfoClient.getTokeninfo(token.AccessToken, ctx)
	if err != nil {
		if err != errInvalidToken {
			ctx.Logger().Errorf("Failed to call tokeninfo: %v.", err)
		}
		loginRedirect(ctx, f.config)
		return
	}

	err = f.setupToken(token, tokeninfo, ctx)
	if err != nil {
		ctx.Logger().Errorf("Failed to create token container: %v.", err)
		loginRedirect(ctx, f.config)
		return
	}
}

func (f *grantFilter) Response(ctx filters.FilterContext) {
	// If the token was refreshed in this request flow,
	// we want to send an updated cookie. If it wasn't, the
	// users will still have their old cookie and we do not
	// need to send it again and this function can exit early.
	token, ok := ctx.StateBag()[refreshedTokenKey].(*oauth2.Token)
	if !ok {
		return
	}

	c, err := createCookie(f.config, ctx.Request().Host, token)
	if err != nil {
		ctx.Logger().Errorf("Failed to generate cookie: %v.", err)
		return
	}

	ctx.Response().Header.Add("Set-Cookie", c.String())
}
