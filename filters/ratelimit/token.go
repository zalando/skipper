package ratelimit

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimitbypass"
)

type tokenGenSpec struct {
	validator *ratelimitbypass.BypassValidator
}

type tokenValidateSpec struct {
	validator *ratelimitbypass.BypassValidator
}

type tokenGenFilter struct {
	validator *ratelimitbypass.BypassValidator
}

type tokenValidateFilter struct {
	validator *ratelimitbypass.BypassValidator
}

// NewRatelimitBypassGenerateToken creates a filter that generates bypass tokens
func NewRatelimitBypassGenerateToken(secretKey string, tokenExpiry time.Duration) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:   secretKey,
		TokenExpiry: tokenExpiry,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &tokenGenSpec{
		validator: validator,
	}
}

// NewRatelimitBypassValidateToken creates a filter that validates bypass tokens
func NewRatelimitBypassValidateToken(secretKey string, tokenExpiry time.Duration, bypassHeader string) filters.Spec {
	config := ratelimitbypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  tokenExpiry,
		BypassHeader: bypassHeader,
	}
	validator := ratelimitbypass.NewBypassValidator(config)

	return &tokenValidateSpec{
		validator: validator,
	}
}

func (s *tokenGenSpec) Name() string {
	return "ratelimitBypassGenerateToken"
}

func (s *tokenGenSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tokenGenFilter{
		validator: s.validator,
	}, nil
}

func (s *tokenValidateSpec) Name() string {
	return "ratelimitBypassValidateToken"
}

func (s *tokenValidateSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tokenValidateFilter{
		validator: s.validator,
	}, nil
}

// Request generates a bypass token and returns it in the response
func (f *tokenGenFilter) Request(ctx filters.FilterContext) {
	token, err := f.validator.GenerateToken()
	if err != nil {
		ctx.Logger().Errorf("Failed to generate bypass token: %v", err)
		ctx.Serve(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		})
		return
	}

	response := map[string]interface{}{
		"token":      token,
		"expires_in": int(f.validator.GetConfig().TokenExpiry.Seconds()),
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		ctx.Logger().Errorf("Failed to marshal token response: %v", err)
		ctx.Serve(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		})
		return
	}

	ctx.Serve(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
	})
}

func (*tokenGenFilter) Response(filters.FilterContext) {}

// Request validates a bypass token and sets response accordingly
func (f *tokenValidateFilter) Request(ctx filters.FilterContext) {
	isValid := f.validator.ValidateToken(ctx.Request())

	response := map[string]interface{}{
		"valid": isValid,
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		ctx.Logger().Errorf("Failed to marshal validation response: %v", err)
		ctx.Serve(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		})
		return
	}

	statusCode := http.StatusOK
	if !isValid {
		statusCode = http.StatusUnauthorized
	}

	ctx.Serve(&http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
	})
}

func (*tokenValidateFilter) Response(filters.FilterContext) {}
