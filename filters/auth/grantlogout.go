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

func (s *grantLogoutSpec) CreateFilter([]any) (filters.Filter, error) {
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

	token, err := f.config.GrantCookieEncoder.Read(req)
	if err != nil {
		unauthorized(
			ctx,
			"",
			missingToken,
			req.Host,
			fmt.Sprintf("No token cookie %v in request.", f.config.TokenCookieName))
		return
	}

	if token.AccessToken == "" && token.RefreshToken == "" {
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
	if token.AccessToken != "" {
		accessTokenRevokeError = f.revokeTokenType(authConfig, accessTokenType, token.AccessToken)
		if accessTokenRevokeError != nil {
			ctx.Logger().Errorf("%v", accessTokenRevokeError)
		}
	}

	if token.RefreshToken != "" {
		refreshTokenRevokeError = f.revokeTokenType(authConfig, refreshTokenType, token.RefreshToken)
		if refreshTokenRevokeError != nil {
			ctx.Logger().Errorf("%v", refreshTokenRevokeError)
		}
	}

	if refreshTokenRevokeError != nil || accessTokenRevokeError != nil {
		serverError(ctx)
	}
}

func (f *grantLogoutFilter) Response(ctx filters.FilterContext) {
	cookies, err := f.config.GrantCookieEncoder.Update(ctx.Request(), nil)
	if err != nil {
		ctx.Logger().Errorf("Failed to delete cookies: %v.", err)
		return
	}
	for _, c := range cookies {
		ctx.Response().Header.Add("Set-Cookie", c.String())
	}
}
