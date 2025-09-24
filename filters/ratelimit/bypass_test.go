package ratelimit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/ratelimit"
)

type mockBypassProvider struct {
	limiter *mockLimit
}

type mockLimit struct {
	allowCount int
	retryAfter int
}

func (m *mockBypassProvider) get(s ratelimit.Settings) limit {
	return m.limiter
}

func (m *mockLimit) Allow(ctx context.Context, s string) bool {
	m.allowCount++
	return m.allowCount <= 2 // Allow first 2 requests, then deny
}

func (m *mockLimit) RetryAfter(s string) int {
	return m.retryAfter
}

func TestClientRatelimitWithBypass_TokenBypass(t *testing.T) {
	provider := &mockBypassProvider{
		limiter: &mockLimit{retryAfter: 60},
	}

	spec := NewClientRatelimitWithBypass(
		provider,
		"X-RateLimit-Bypass",
		"test-secret",
		time.Minute*5,
		[]string{},
	)

	filter, err := spec.CreateFilter([]interface{}{2, "1m"})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	// Generate a bypass token
	bypassFilter := filter.(*bypassFilter)
	token, err := bypassFilter.validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Test request with bypass token - should be allowed even when rate limit is exceeded
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-RateLimit-Bypass", token)
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := &filtertest.Context{
		FRequest: req,
	}

	// All calls should succeed because of bypass token
	filter.Request(ctx)
	if ctx.FServed {
		t.Fatal("First request with bypass token should not be rate limited")
	}

	filter.Request(ctx)
	if ctx.FServed {
		t.Fatal("Second request with bypass token should not be rate limited")
	}

	filter.Request(ctx)
	if ctx.FServed {
		t.Fatal("Third request with bypass token should not be rate limited")
	}

	// Test without bypass token - should be rate limited on third call
	reqNoBypass, _ := http.NewRequest("GET", "/test", nil)
	reqNoBypass.RemoteAddr = "127.0.0.1:12345"

	ctxNoBypass := &filtertest.Context{
		FRequest: reqNoBypass,
	}

	provider.limiter.allowCount = 0 // Reset counter
	filter.Request(ctxNoBypass)
	if ctxNoBypass.FServed {
		t.Fatal("First request without bypass should not be rate limited")
	}

	filter.Request(ctxNoBypass)
	if ctxNoBypass.FServed {
		t.Fatal("Second request without bypass should not be rate limited")
	}

	filter.Request(ctxNoBypass)
	if !ctxNoBypass.FServed {
		t.Fatal("Third request without bypass should be rate limited")
	}
}

func TestClientRatelimitWithBypass_IPWhitelist(t *testing.T) {
	provider := &mockBypassProvider{
		limiter: &mockLimit{retryAfter: 60},
	}

	spec := NewClientRatelimitWithBypass(
		provider,
		"X-RateLimit-Bypass",
		"test-secret",
		time.Minute*5,
		[]string{"127.0.0.1"},
	)

	filter, err := spec.CreateFilter([]interface{}{1, "1m"})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	// Test request from whitelisted IP
	ctx := &filtertest.Context{
		FRequest: &http.Request{
			RemoteAddr: "127.0.0.1:12345",
			Header:     http.Header{},
		},
	}

	// Even though limit is 1, whitelisted IP should bypass
	provider.limiter.allowCount = 10 // Set high to ensure rate limiting would trigger
	filter.Request(ctx)
	if ctx.FServed {
		t.Fatal("Request from whitelisted IP should not be rate limited")
	}

	// Test request from non-whitelisted IP
	ctxNonWhitelisted := &filtertest.Context{
		FRequest: &http.Request{
			RemoteAddr: "10.0.0.1:12345",
			Header:     http.Header{},
		},
	}

	filter.Request(ctxNonWhitelisted)
	if !ctxNonWhitelisted.FServed {
		t.Fatal("Request from non-whitelisted IP should be rate limited")
	}
}

func TestRatelimitBypassTokenGeneration(t *testing.T) {
	spec := NewRatelimitBypassGenerateToken("test-secret", time.Minute*5)

	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create token generation filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{},
	}

	filter.Request(ctx)

	if !ctx.FServed {
		t.Fatal("Token generation filter should serve response")
	}

	if ctx.FResponse == nil {
		t.Fatal("Token generation filter should set response")
	}

	if ctx.FResponse.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", ctx.FResponse.StatusCode)
	}

	if ctx.FResponse.Body == nil {
		t.Fatal("Token generation filter should set response body")
	}
}

func TestRatelimitBypassTokenValidation_ValidToken(t *testing.T) {
	secretKey := "test-secret"
	tokenExpiry := time.Minute * 5

	// First generate a token
	genSpec := NewRatelimitBypassGenerateToken(secretKey, tokenExpiry)
	genFilter, _ := genSpec.CreateFilter([]interface{}{})
	genCtx := &filtertest.Context{
		FRequest: &http.Request{},
	}
	genFilter.Request(genCtx)

	// Extract token from response (simplified - in real test you'd parse JSON)
	// For this test, we'll generate it directly
	genF := genFilter.(*tokenGenFilter)
	token, _ := genF.validator.GenerateToken()

	// Now validate the token
	validateSpec := NewRatelimitBypassValidateToken(secretKey, tokenExpiry, "X-RateLimit-Bypass")
	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create token validation filter: %v", err)
	}

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-RateLimit-Bypass", token)
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := &filtertest.Context{
		FRequest: req,
	}

	validateFilter.Request(ctx)

	if !ctx.FServed {
		t.Fatal("Token validation filter should serve response")
	}

	if ctx.FResponse.StatusCode != http.StatusOK {
		t.Fatalf("Valid token should return 200, got %d", ctx.FResponse.StatusCode)
	}
}

func TestRatelimitBypassTokenValidation_InvalidToken(t *testing.T) {
	spec := NewRatelimitBypassValidateToken("test-secret", time.Minute*5, "X-RateLimit-Bypass")

	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create token validation filter: %v", err)
	}

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-RateLimit-Bypass", "invalid-token")
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := &filtertest.Context{
		FRequest: req,
	}

	filter.Request(ctx)

	if !ctx.FServed {
		t.Fatal("Token validation filter should serve response")
	}

	if ctx.FResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Invalid token should return 401, got %d", ctx.FResponse.StatusCode)
	}
}

func TestBypassFilterNames(t *testing.T) {
	provider := &mockBypassProvider{
		limiter: &mockLimit{},
	}

	testCases := []struct {
		spec         filters.Spec
		expectedName string
	}{
		{
			NewClientRatelimitWithBypass(provider, "X-Bypass", "secret", time.Minute, []string{}),
			"clientRatelimitWithBypass",
		},
		{
			NewRatelimitWithBypass(provider, "X-Bypass", "secret", time.Minute, []string{}),
			"ratelimitWithBypass",
		},
		{
			NewClusterClientRatelimitWithBypass(provider, "X-Bypass", "secret", time.Minute, []string{}),
			"clusterClientRatelimitWithBypass",
		},
		{
			NewClusterRatelimitWithBypass(provider, "X-Bypass", "secret", time.Minute, []string{}),
			"clusterRatelimitWithBypass",
		},
		{
			NewDisableRatelimitWithBypass(provider, "X-Bypass", "secret", time.Minute, []string{}),
			"disableRatelimitWithBypass",
		},
		{
			NewRatelimitBypassGenerateToken("secret", time.Minute),
			"ratelimitBypassGenerateToken",
		},
		{
			NewRatelimitBypassValidateToken("secret", time.Minute, "X-Bypass"),
			"ratelimitBypassValidateToken",
		},
	}

	for _, tc := range testCases {
		if tc.spec.Name() != tc.expectedName {
			t.Errorf("Expected name %s, got %s", tc.expectedName, tc.spec.Name())
		}
	}
}
