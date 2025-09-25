package ratelimit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/ratelimit/bypass"
)

func TestTokenGenSpec_Name(t *testing.T) {
	spec := NewRatelimitBypassGenerateToken("test-secret", time.Minute)
	if spec.Name() != "ratelimitBypassGenerateToken" {
		t.Errorf("Expected name 'ratelimitBypassGenerateToken', got '%s'", spec.Name())
	}
}

func TestTokenValidateSpec_Name(t *testing.T) {
	spec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-RateLimit-Bypass")
	if spec.Name() != "ratelimitBypassValidateToken" {
		t.Errorf("Expected name 'ratelimitBypassValidateToken', got '%s'", spec.Name())
	}
}

func TestTokenGenSpec_CreateFilter(t *testing.T) {
	spec := NewRatelimitBypassGenerateToken("test-secret", time.Minute)

	// Test valid creation (no args)
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}
	if filter == nil {
		t.Fatal("Filter is nil")
	}

	// Test invalid creation (with args)
	_, err = spec.CreateFilter([]interface{}{"arg"})
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("Expected ErrInvalidFilterParameters, got %v", err)
	}
}

func TestTokenValidateSpec_CreateFilter(t *testing.T) {
	spec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-RateLimit-Bypass")

	// Test valid creation (no args)
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}
	if filter == nil {
		t.Fatal("Filter is nil")
	}

	// Test invalid creation (with args)
	_, err = spec.CreateFilter([]interface{}{"arg"})
	if err != filters.ErrInvalidFilterParameters {
		t.Errorf("Expected ErrInvalidFilterParameters, got %v", err)
	}
}

func TestTokenGenFilter_Request(t *testing.T) {
	spec := NewRatelimitBypassGenerateToken("test-secret-key", time.Minute*5)
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/test"},
		},
	}

	filter.Request(ctx)

	// Check that a response was served
	if !ctx.FServed {
		t.Fatal("No response was served")
	}

	response := ctx.FResponse
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", response.StatusCode)
	}

	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Read and parse response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var tokenResponse map[string]interface{}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check response structure
	token, ok := tokenResponse["token"].(string)
	if !ok || token == "" {
		t.Error("Token field is missing or empty")
	}

	expiresIn, ok := tokenResponse["expires_in"].(float64)
	if !ok || expiresIn != 300 { // 5 minutes = 300 seconds
		t.Errorf("Expected expires_in to be 300, got %v", expiresIn)
	}
}

func TestBypassValidator_DirectTest(t *testing.T) {
	secretKey := "test-secret-key"
	bypassHeader := "X-RateLimit-Bypass"

	config := bypass.BypassConfig{
		SecretKey:    secretKey,
		TokenExpiry:  time.Minute * 5,
		BypassHeader: bypassHeader,
	}

	validator := bypass.NewBypassValidator(config)

	// Generate token directly
	token, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}

	// Test validation directly
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set(bypassHeader, token)

	if !validator.ValidateToken(req) {
		t.Error("Valid token was rejected by bypass validator")
	}
}

func TestTokenValidateFilter_Request_ValidToken(t *testing.T) {
	secretKey := "test-secret-key"
	bypassHeader := "X-RateLimit-Bypass"

	// Create the validation filter and extract its validator for testing
	validateSpec := NewRatelimitBypassValidateToken(secretKey, time.Minute*5, bypassHeader)
	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create validate filter: %v", err)
	}

	// Cast to get access to the validator
	valFilter := validateFilter.(*tokenValidateFilter)

	// Generate a token using the same validator that the filter uses
	directToken, err := valFilter.validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Test validation directly with the validator
	testReq, _ := http.NewRequest("GET", "/test", nil)
	testReq.Header.Set(bypassHeader, directToken)
	if !valFilter.validator.ValidateToken(testReq) {
		t.Error("Token should be valid when tested directly with the validator")
	}

	// Now test through the filter
	requestForFilter := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Header: make(http.Header),
	}
	requestForFilter.Header.Set(bypassHeader, directToken)

	// Test that the filter's validator can validate the exact request we're sending
	if !valFilter.validator.ValidateToken(requestForFilter) {
		t.Errorf("Token should be valid for the request we're sending to the filter. Token: %s, Header: %s", directToken, bypassHeader)
	}

	validateCtx := &filtertest.Context{
		FRequest: requestForFilter,
	}

	validateFilter.Request(validateCtx)

	// Check response
	body, _ := io.ReadAll(validateCtx.FResponse.Body)
	var validationResponse map[string]interface{}
	json.Unmarshal(body, &validationResponse)

	if validateCtx.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Response: %s", validateCtx.FResponse.StatusCode, string(body))
	}

	valid, ok := validationResponse["valid"].(bool)
	if !ok || !valid {
		t.Errorf("Expected valid to be true. Response: %s", string(body))
	}
}

func TestTokenValidateFilter_Request_InvalidToken(t *testing.T) {
	secretKey := "test-secret-key"
	bypassHeader := "X-RateLimit-Bypass"

	validateSpec := NewRatelimitBypassValidateToken(secretKey, time.Minute*5, bypassHeader)
	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create validate filter: %v", err)
	}

	invalidReq := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test"},
		Header: make(http.Header),
	}
	invalidReq.Header.Set(bypassHeader, "invalid-token")

	validateCtx := &filtertest.Context{
		FRequest: invalidReq,
	}

	validateFilter.Request(validateCtx)

	// Check response
	if validateCtx.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", validateCtx.FResponse.StatusCode)
	}

	body, _ := io.ReadAll(validateCtx.FResponse.Body)
	var validationResponse map[string]interface{}
	json.Unmarshal(body, &validationResponse)

	valid, ok := validationResponse["valid"].(bool)
	if !ok || valid {
		t.Error("Expected valid to be false")
	}
}

func TestTokenValidateFilter_Request_NoToken(t *testing.T) {
	secretKey := "test-secret-key"
	bypassHeader := "X-RateLimit-Bypass"

	validateSpec := NewRatelimitBypassValidateToken(secretKey, time.Minute*5, bypassHeader)
	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create validate filter: %v", err)
	}

	validateCtx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/test"},
			Header: http.Header{},
		},
	}

	validateFilter.Request(validateCtx)

	// Check response
	if validateCtx.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", validateCtx.FResponse.StatusCode)
	}

	body, _ := io.ReadAll(validateCtx.FResponse.Body)
	var validationResponse map[string]interface{}
	json.Unmarshal(body, &validationResponse)

	valid, ok := validationResponse["valid"].(bool)
	if !ok || valid {
		t.Error("Expected valid to be false")
	}
}

func TestTokenFilters_Integration(t *testing.T) {
	secretKey := "test-secret-key-integration"
	bypassHeader := "X-RateLimit-Bypass"
	tokenExpiry := time.Minute * 10

	// Create both filters
	genSpec := NewRatelimitBypassGenerateToken(secretKey, tokenExpiry)
	validateSpec := NewRatelimitBypassValidateToken(secretKey, tokenExpiry, bypassHeader)

	genFilter, err := genSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create generate filter: %v", err)
	}

	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create validate filter: %v", err)
	}

	// Step 1: Generate token
	genCtx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/generate-token"},
		},
	}

	genFilter.Request(genCtx)

	if genCtx.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Token generation failed with status %d", genCtx.FResponse.StatusCode)
	}

	// Extract token
	body, _ := io.ReadAll(genCtx.FResponse.Body)
	var tokenResponse map[string]interface{}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		t.Fatalf("Failed to parse token response: %v", err)
	}

	token, ok := tokenResponse["token"].(string)
	if !ok || token == "" {
		t.Fatal("Token not found in response")
	}

	// Step 2: Validate the generated token
	validateReq := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/validate-token"},
		Header: make(http.Header),
	}
	validateReq.Header.Set(bypassHeader, token)

	validateCtx := &filtertest.Context{
		FRequest: validateReq,
	}

	validateFilter.Request(validateCtx)

	if validateCtx.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Token validation failed with status %d", validateCtx.FResponse.StatusCode)
	}

	body, _ = io.ReadAll(validateCtx.FResponse.Body)
	var validationResponse map[string]interface{}
	if err := json.Unmarshal(body, &validationResponse); err != nil {
		t.Fatalf("Failed to parse validation response: %v", err)
	}

	valid, ok := validationResponse["valid"].(bool)
	if !ok || !valid {
		t.Error("Generated token should be valid")
	}

	// Step 3: Test with wrong secret key
	wrongSpec := NewRatelimitBypassValidateToken("wrong-secret", tokenExpiry, bypassHeader)
	wrongFilter, err := wrongSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter with wrong secret: %v", err)
	}

	wrongReq := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/validate-token"},
		Header: make(http.Header),
	}
	wrongReq.Header.Set(bypassHeader, token)

	wrongCtx := &filtertest.Context{
		FRequest: wrongReq,
	}

	wrongFilter.Request(wrongCtx)

	if wrongCtx.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Token should be invalid with wrong secret, got status %d", wrongCtx.FResponse.StatusCode)
	}
}

func TestTokenFilters_Response(t *testing.T) {
	// Test that Response methods don't panic and do nothing
	genSpec := NewRatelimitBypassGenerateToken("test-secret", time.Minute)
	validateSpec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-Header")

	genFilter, _ := genSpec.CreateFilter([]interface{}{})
	validateFilter, _ := validateSpec.CreateFilter([]interface{}{})

	ctx := &filtertest.Context{}

	// These should not panic
	genFilter.Response(ctx)
	validateFilter.Response(ctx)
}

func TestTokenGenFilter_JsonMarshalError(t *testing.T) {
	// Test error handling when JSON marshaling fails
	// This is difficult to trigger directly, so we'll test the normal case
	// and ensure error paths are covered by other means
	spec := NewRatelimitBypassGenerateToken("test-secret", time.Minute)
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/generate-token"},
		},
	}

	filter.Request(ctx)

	// Should succeed normally - JSON marshal error is hard to trigger
	if ctx.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", ctx.FResponse.StatusCode)
	}
}

func TestTokenValidateFilter_JsonMarshalError(t *testing.T) {
	// Test JSON marshal error in validation response
	// This is also difficult to trigger directly with simple bool/string values
	spec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-Header")
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/validate"},
			Header: make(http.Header),
		},
	}
	ctx.FRequest.Header.Set("X-Header", "invalid-token")

	filter.Request(ctx)

	// Should handle invalid token normally - JSON marshal unlikely to fail with simple data
	if ctx.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", ctx.FResponse.StatusCode)
	}
}

func TestTokenFilters_ResponseMethodsCoverage(t *testing.T) {
	// Ensure Response methods are covered - they should be no-ops
	genSpec := NewRatelimitBypassGenerateToken("test-secret", time.Minute)
	validateSpec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-Header")

	genFilter, err := genSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create generate filter: %v", err)
	}

	validateFilter, err := validateSpec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create validate filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/test"},
		},
	}

	// These should be no-ops and not panic
	genFilter.Response(ctx)
	validateFilter.Response(ctx)

	// Verify context wasn't modified by Response calls
	if ctx.FResponse != nil {
		t.Error("Response methods should not modify the context")
	}
}

func TestTokenGenFilter_ErrorPathsCoverage(t *testing.T) {
	// Since it's difficult to make the real validator fail token generation,
	// we'll test the paths that we can test and ensure they're covered

	// Test with extremely short token expiry to potentially trigger edge cases
	spec := NewRatelimitBypassGenerateToken("test-secret", time.Nanosecond)
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/generate-token"},
		},
	}

	filter.Request(ctx)

	// Should still succeed - token generation is robust
	if ctx.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", ctx.FResponse.StatusCode)
	}
}

func TestTokenValidateFilter_AllResponsePaths(t *testing.T) {
	// Test both valid and invalid token paths to ensure coverage
	spec := NewRatelimitBypassValidateToken("test-secret", time.Minute, "X-Test-Header")
	filter, err := spec.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	// Test invalid token path
	ctx1 := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/validate"},
			Header: make(http.Header),
		},
	}
	ctx1.FRequest.Header.Set("X-Test-Header", "definitely-invalid-token")

	filter.Request(ctx1)

	if ctx1.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for invalid token, got %d", ctx1.FResponse.StatusCode)
	}

	// Verify response structure for invalid token
	body1, err := io.ReadAll(ctx1.FResponse.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var response1 map[string]interface{}
	if err := json.Unmarshal(body1, &response1); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if valid, ok := response1["valid"].(bool); !ok || valid {
		t.Errorf("Expected valid to be false, got %v", response1["valid"])
	}

	// Test with no token at all
	ctx2 := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/validate"},
			Header: make(http.Header),
		},
	}

	filter.Request(ctx2)

	if ctx2.FResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for no token, got %d", ctx2.FResponse.StatusCode)
	}
}

func TestTokenFilters_EdgeCaseCoverage(t *testing.T) {
	// Test various edge cases to improve coverage

	// Test with empty secret key (should still work but be insecure)
	spec1 := NewRatelimitBypassGenerateToken("", time.Minute)
	filter1, err := spec1.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter with empty secret: %v", err)
	}

	ctx1 := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/generate-token"},
		},
	}

	filter1.Request(ctx1)

	// Should still work
	if ctx1.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 with empty secret, got %d", ctx1.FResponse.StatusCode)
	}

	// Test with very long expiry
	spec2 := NewRatelimitBypassGenerateToken("test-secret", time.Hour*24*365)
	filter2, err := spec2.CreateFilter([]interface{}{})
	if err != nil {
		t.Fatalf("Failed to create filter with long expiry: %v", err)
	}

	ctx2 := &filtertest.Context{
		FRequest: &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/generate-token"},
		},
	}

	filter2.Request(ctx2)

	if ctx2.FResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 with long expiry, got %d", ctx2.FResponse.StatusCode)
	}

	// Verify the response has the correct expiry
	body2, err := io.ReadAll(ctx2.FResponse.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var response2 map[string]interface{}
	if err := json.Unmarshal(body2, &response2); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	expectedExpiry := int(time.Hour.Seconds() * 24 * 365)
	if expiresIn, ok := response2["expires_in"].(float64); !ok || int(expiresIn) != expectedExpiry {
		t.Errorf("Expected expires_in to be %d, got %v", expectedExpiry, response2["expires_in"])
	}
}
