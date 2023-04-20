package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

const (
	// Deprecated, use filters.GrantLogoutName instead
	GrantLogoutName = filters.GrantLogoutName

	revokeTokenKey          = "token"
	revokeTokenTypeKey      = "token_type_hint"
	refreshTokenType        = "refresh_token"
	accessTokenType         = "access_token"
	errUnsupportedTokenType = "unsupported_token_type"
)

type grantLogoutSpec struct {
	config *OAuthConfig
}

type grantLogoutFilter struct {
	config *OAuthConfig
}

type revokeErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (*grantLogoutSpec) Name() string { return filters.GrantLogoutName }

func (s *grantLogoutSpec) CreateFilter([]interface{}) (filters.Filter, error) {
	return &grantLogoutFilter{
		config: s.config,
	}, nil
}

func responseToError(responseData []byte, statusCode int, tokenType string) error {
	var errorResponse revokeErrorResponse
	err := json.Unmarshal(responseData, &errorResponse)

	if err != nil {
		return err
	}

	if errorResponse.Error == errUnsupportedTokenType && tokenType == accessTokenType {
		// Provider does not support revoking access tokens, which can happen according to RFC 7009.
		// In that case this is not really an error.
		return nil
	}
	return fmt.Errorf(
		"%s revocation failed: %d %s: %s",
		tokenType,
		statusCode,
		errorResponse.Error,
		errorResponse.ErrorDescription,
	)
}

func (f *grantLogoutFilter) revokeTokenType(c *oauth2.Config, tokenType string, token string) error {
	revokeURL, err := url.Parse(f.config.RevokeTokenURL)
	if err != nil {
		return err
	}

	query := revokeURL.Query()
	for k, v := range f.config.AuthURLParameters {
		query.Set(k, v)
	}
	revokeURL.RawQuery = query.Encode()

	body := url.Values{}
	body.Add(revokeTokenKey, token)
	body.Add(revokeTokenTypeKey, tokenType)

	revokeRequest, err := http.NewRequest(
		"POST",
		revokeURL.String(),
		strings.NewReader(body.Encode()))

	if err != nil {
		return err
	}

	revokeRequest.SetBasicAuth(c.ClientID, c.ClientSecret)
	revokeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	revokeResponse, err := f.config.AuthClient.Do(revokeRequest)
	if err != nil {
		return err
	}
	defer revokeResponse.Body.Close()

	buf, err := io.ReadAll(revokeResponse.Body)
	if err != nil {
		return err
	}

	if revokeResponse.StatusCode == 400 {
		return responseToError(buf, revokeResponse.StatusCode, tokenType)
	} else if revokeResponse.StatusCode != 200 {
		return fmt.Errorf(
			"%s revocation failed: %d",
			tokenType,
			revokeResponse.StatusCode,
		)
	}

	return nil
}

func (f *grantLogoutFilter) Request(ctx filters.FilterContext) {
	if f.config.RevokeTokenURL == "" {
		return
	}

	req := ctx.Request()

	c, err := extractCookie(req, f.config)
	if err != nil {
		unauthorized(
			ctx,
			"",
			missingToken,
			req.Host,
			fmt.Sprintf("No token cookie %v in request.", f.config.TokenCookieName))
		return
	}

	if c.AccessToken == "" && c.RefreshToken == "" {
		unauthorized(
			ctx,
			"",
			missingToken,
			req.Host,
			fmt.Sprintf("Token cookie %v has no tokens.", f.config.TokenCookieName))
		return
	}

	authConfig, err := f.config.GetConfig(req)
	if err != nil {
		serverError(ctx)
		return
	}

	var accessTokenRevokeError, refreshTokenRevokeError error
	if c.AccessToken != "" {
		accessTokenRevokeError = f.revokeTokenType(authConfig, accessTokenType, c.AccessToken)
		if accessTokenRevokeError != nil {
			ctx.Logger().Errorf("%v", accessTokenRevokeError)
		}
	}

	if c.RefreshToken != "" {
		refreshTokenRevokeError = f.revokeTokenType(authConfig, refreshTokenType, c.RefreshToken)
		if refreshTokenRevokeError != nil {
			ctx.Logger().Errorf("%v", refreshTokenRevokeError)
		}
	}

	if refreshTokenRevokeError != nil || accessTokenRevokeError != nil {
		serverError(ctx)
	}
}

func (f *grantLogoutFilter) Response(ctx filters.FilterContext) {
	deleteCookie := createDeleteCookie(f.config, ctx.Request().Host)
	ctx.Response().Header.Add("Set-Cookie", deleteCookie.String())
}
