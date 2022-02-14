package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
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
	config OAuthConfig
}

type grantLogoutFilter struct {
	config OAuthConfig
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

func (f *grantLogoutFilter) getBasicAuthCredentials() (string, string, error) {
	clientID := f.config.GetClientID()
	if clientID == "" {
		return "", "", errors.New("failed to create token revoke auth header: no client ID")
	}

	clientSecret := f.config.GetClientSecret()
	if clientSecret == "" {
		return "", "", errors.New("failed to create token revoke auth header: no client secret")
	}

	return clientID, clientSecret, nil
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

func (f *grantLogoutFilter) revokeTokenType(tokenType string, token string) error {
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

	clientId, clientSecret, err := f.getBasicAuthCredentials()
	if err != nil {
		return err
	}

	revokeRequest.SetBasicAuth(clientId, clientSecret)
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

	var accessTokenRevokeError, refreshTokenRevokeError error
	if c.AccessToken != "" {
		accessTokenRevokeError = f.revokeTokenType(accessTokenType, c.AccessToken)
		if accessTokenRevokeError != nil {
			log.Error(accessTokenRevokeError)
		}
	}

	if c.RefreshToken != "" {
		refreshTokenRevokeError = f.revokeTokenType(refreshTokenType, c.RefreshToken)
		if refreshTokenRevokeError != nil {
			log.Error(refreshTokenRevokeError)
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
