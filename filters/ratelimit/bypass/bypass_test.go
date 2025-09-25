package bypass

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBypassValidator_GenerateAndValidateToken(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"127.0.0.1", "192.168.1.0/24"},
	}

	validator := NewBypassValidator(config)

	// Test token generation
	token, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}

	// Test token validation with valid token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set(config.BypassHeader, token)

	if !validator.ValidateToken(req) {
		t.Fatal("Valid token was rejected")
	}

	// Test token validation with invalid token
	req.Header.Set(config.BypassHeader, "invalid-token")
	if validator.ValidateToken(req) {
		t.Fatal("Invalid token was accepted")
	}

	// Test empty token
	req.Header.Del(config.BypassHeader)
	if validator.ValidateToken(req) {
		t.Fatal("Empty token was accepted")
	}
}

func TestBypassValidator_IPWhitelist(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"127.0.0.1", "192.168.1.0/24"},
	}

	validator := NewBypassValidator(config)

	// Test whitelisted IP
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !validator.IsWhitelisted(req) {
		t.Fatal("Whitelisted IP was not recognized")
	}

	// Test IP in whitelisted range
	req.RemoteAddr = "192.168.1.100:12345"
	if !validator.IsWhitelisted(req) {
		t.Fatal("IP in whitelisted range was not recognized")
	}

	// Test non-whitelisted IP
	req.RemoteAddr = "10.0.0.1:12345"
	if validator.IsWhitelisted(req) {
		t.Fatal("Non-whitelisted IP was accepted")
	}

	// Test with X-Forwarded-For header
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	if !validator.IsWhitelisted(req) {
		t.Fatal("Whitelisted IP in X-Forwarded-For was not recognized")
	}
}

func TestBypassValidator_ShouldBypass(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"127.0.0.1"},
	}

	validator := NewBypassValidator(config)

	// Test bypass with whitelisted IP
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !validator.ShouldBypass(req) {
		t.Fatal("Request from whitelisted IP should bypass")
	}

	// Test bypass with valid token
	req.RemoteAddr = "10.0.0.1:12345"
	token, _ := validator.GenerateToken()
	req.Header.Set(config.BypassHeader, token)
	if !validator.ShouldBypass(req) {
		t.Fatal("Request with valid token should bypass")
	}

	// Test no bypass with invalid token and non-whitelisted IP
	req.Header.Set(config.BypassHeader, "invalid-token")
	if validator.ShouldBypass(req) {
		t.Fatal("Request with invalid token and non-whitelisted IP should not bypass")
	}

	// Test no bypass without token or whitelist
	req.Header.Del(config.BypassHeader)
	if validator.ShouldBypass(req) {
		t.Fatal("Request without token or whitelist should not bypass")
	}
}

func TestBypassValidator_ExpiredToken(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Second * 1, // 1 second expiry
		BypassHeader: "X-RateLimit-Bypass",
	}

	validator := NewBypassValidator(config)

	// Generate token
	token, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Verify token is valid immediately
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set(config.BypassHeader, token)

	if !validator.ValidateToken(req) {
		t.Fatal("Freshly generated token should be valid")
	}

	// Wait for token to expire (wait longer than expiry time)
	time.Sleep(time.Second * 2)

	// Test expired token
	if validator.ValidateToken(req) {
		t.Fatal("Expired token was accepted")
	}
}

func TestParseIPWhitelist(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"127.0.0.1", []string{"127.0.0.1"}},
		{"127.0.0.1,192.168.1.0/24", []string{"127.0.0.1", "192.168.1.0/24"}},
		{"127.0.0.1, 192.168.1.0/24 , 10.0.0.1", []string{"127.0.0.1", "192.168.1.0/24", "10.0.0.1"}},
		{" , , ", nil},
	}

	for _, tc := range testCases {
		result := ParseIPWhitelist(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("For input %q, expected %v, got %v", tc.input, tc.expected, result)
			continue
		}

		for i, expected := range tc.expected {
			if result[i] != expected {
				t.Errorf("For input %q, expected %v, got %v", tc.input, tc.expected, result)
				break
			}
		}
	}
}

func TestBypassValidator_CookieSupport(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		BypassCookie: "bypass-token",
		IPWhitelist:  []string{},
	}

	validator := NewBypassValidator(config)

	// Generate a valid token
	token, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Test bypass with cookie
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	cookie := &http.Cookie{
		Name:  config.BypassCookie,
		Value: token,
	}
	req.AddCookie(cookie)

	if !validator.ValidateToken(req) {
		t.Fatal("Valid token in cookie was not accepted")
	}

	if !validator.ShouldBypass(req) {
		t.Fatal("Request with valid cookie token should bypass")
	}

	// Test bypass with both header and cookie (header should take precedence)
	req.Header.Set(config.BypassHeader, "invalid-token")
	if validator.ValidateToken(req) {
		t.Fatal("Invalid token in header should not be accepted even with valid cookie")
	}

	// Test bypass with only cookie when header is empty
	req.Header.Del(config.BypassHeader)
	if !validator.ValidateToken(req) {
		t.Fatal("Valid token in cookie should be accepted when header is empty")
	}

	// Test with invalid cookie token
	invalidCookie := &http.Cookie{
		Name:  config.BypassCookie,
		Value: "invalid-token",
	}
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.AddCookie(invalidCookie)

	if validator.ValidateToken(req2) {
		t.Fatal("Invalid token in cookie should not be accepted")
	}

	if validator.ShouldBypass(req2) {
		t.Fatal("Request with invalid cookie token should not bypass")
	}

	// Test without bypass cookie configuration
	configNoCookie := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		BypassCookie: "", // No cookie configured
		IPWhitelist:  []string{},
	}
	validatorNoCookie := NewBypassValidator(configNoCookie)

	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "10.0.0.1:12345"
	req3.AddCookie(cookie) // Same valid token in cookie

	if validatorNoCookie.ValidateToken(req3) {
		t.Fatal("Token in cookie should be ignored when no bypass cookie is configured")
	}
}

func TestBypassValidator_GetConfig(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		BypassCookie: "bypass-token",
		IPWhitelist:  []string{"127.0.0.1"},
	}

	validator := NewBypassValidator(config)
	retrievedConfig := validator.GetConfig()

	if retrievedConfig.SecretKey != config.SecretKey {
		t.Errorf("Expected SecretKey %s, got %s", config.SecretKey, retrievedConfig.SecretKey)
	}
	if retrievedConfig.TokenExpiry != config.TokenExpiry {
		t.Errorf("Expected TokenExpiry %v, got %v", config.TokenExpiry, retrievedConfig.TokenExpiry)
	}
	if retrievedConfig.BypassHeader != config.BypassHeader {
		t.Errorf("Expected BypassHeader %s, got %s", config.BypassHeader, retrievedConfig.BypassHeader)
	}
	if retrievedConfig.BypassCookie != config.BypassCookie {
		t.Errorf("Expected BypassCookie %s, got %s", config.BypassCookie, retrievedConfig.BypassCookie)
	}
}

func TestBypassValidator_IPv6Support(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"::1", "2001:db8::/32"},
	}

	validator := NewBypassValidator(config)

	// Test IPv6 localhost
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[::1]:12345"
	if !validator.IsWhitelisted(req) {
		t.Error("IPv6 localhost should be whitelisted")
	}

	// Test IPv6 range
	req.RemoteAddr = "[2001:db8::1]:12345"
	if !validator.IsWhitelisted(req) {
		t.Error("IPv6 address in range should be whitelisted")
	}

	// Test IPv6 outside range
	req.RemoteAddr = "[2001:db9::1]:12345"
	if validator.IsWhitelisted(req) {
		t.Error("IPv6 address outside range should not be whitelisted")
	}
}

func TestBypassValidator_InvalidIPHandling(t *testing.T) {
	// Test with invalid IP/CIDR in whitelist - should be logged and skipped
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"invalid-ip", "127.0.0.1", "not.a.cidr/999"},
	}

	validator := NewBypassValidator(config)

	// Should still work with valid IP despite invalid entries
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !validator.IsWhitelisted(req) {
		t.Error("Valid IP should still work despite invalid entries in whitelist")
	}
}

func TestBypassValidator_ClientIPHeaders(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"192.168.1.100"},
	}

	validator := NewBypassValidator(config)

	// Test X-Real-IP header
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "192.168.1.100")
	if !validator.IsWhitelisted(req) {
		t.Error("X-Real-IP header should be used for client IP detection")
	}

	// Test X-Forwarded-For with multiple IPs
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.2, 10.0.0.3")
	if !validator.IsWhitelisted(req2) {
		t.Error("X-Forwarded-For header should use first IP for client detection")
	}

	// Test malformed remote address
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "malformed-address"
	if validator.IsWhitelisted(req3) {
		t.Error("Malformed remote address should not be whitelisted")
	}
}

func TestBypassValidator_MalformedTokens(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
	}

	validator := NewBypassValidator(config)

	testCases := []struct {
		name  string
		token string
	}{
		{"Too few parts", "invalid"},
		{"Two parts only", "header.payload"},
		{"Four parts", "header.payload.signature.extra"},
		{"Invalid base64 in payload", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid-base64.signature"},
		{"Invalid JSON in payload", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.aW52YWxpZC1qc29u.signature"},
		{"Invalid base64 signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjk5OTk5OTk5OTksImlhdCI6MX0.invalid-base64"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set(config.BypassHeader, tc.token)

			if validator.ValidateToken(req) {
				t.Errorf("Malformed token should not be valid: %s", tc.name)
			}
		})
	}
}

func TestBypassValidator_EmptyWhitelist(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{},
	}

	validator := NewBypassValidator(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	if validator.IsWhitelisted(req) {
		t.Error("Empty whitelist should not allow any IPs")
	}
}

func TestBypassValidator_WhitelistWithEmptyStrings(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		IPWhitelist:  []string{"", "  ", "127.0.0.1", "", "192.168.1.0/24", "  "},
	}

	validator := NewBypassValidator(config)

	// Should work with valid IPs despite empty strings
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !validator.IsWhitelisted(req) {
		t.Error("Whitelist should work despite empty string entries")
	}
}

func TestBypassValidator_HeaderPriority(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		BypassCookie: "bypass-token",
	}

	validator := NewBypassValidator(config)

	// Generate valid token
	validToken, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Test header takes precedence over cookie
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set(config.BypassHeader, "invalid-header-token")
	req.AddCookie(&http.Cookie{
		Name:  config.BypassCookie,
		Value: validToken,
	})

	// Header should take precedence and be invalid
	if validator.ValidateToken(req) {
		t.Error("Invalid header token should take precedence over valid cookie")
	}

	// Test with empty header - should fall back to cookie
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.AddCookie(&http.Cookie{
		Name:  config.BypassCookie,
		Value: validToken,
	})

	if !validator.ValidateToken(req2) {
		t.Error("Should fall back to cookie when header is empty")
	}
}

func TestBypassValidator_DefaultHeader(t *testing.T) {
	// Test default header when BypassHeader is empty
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "", // Empty - should use default
	}

	validator := NewBypassValidator(config)
	token, err := validator.GenerateToken()
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-RateLimit-Bypass", token) // Default header name

	if !validator.ValidateToken(req) {
		t.Error("Should use default header 'X-RateLimit-Bypass' when BypassHeader is empty")
	}
}

func TestBypassValidator_CookieError(t *testing.T) {
	// Test cookie retrieval error handling
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
		BypassCookie: "bypass-token",
	}

	validator := NewBypassValidator(config)

	// Request with no header and no cookies - should trigger cookie error path
	req, _ := http.NewRequest("GET", "/test", nil)
	// Deliberately not setting the cookie to test the error path

	if validator.ValidateToken(req) {
		t.Error("Should return false when no header and no cookie found")
	}
}

func TestBypassValidator_JWTSignatureErrors(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
	}

	validator := NewBypassValidator(config)

	// Test signature decode error with invalid base64
	req, _ := http.NewRequest("GET", "/test", nil)
	// Create a token with valid header and payload but invalid signature base64
	validHeader := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	validPayload := "eyJleHAiOjk5OTk5OTk5OTksImlhdCI6MSwiU3ViIjoicmF0ZWxpbWl0LWJ5cGFzcyJ9"
	invalidSignature := "invalid-base64-chars!@#$%"
	malformedToken := validHeader + "." + validPayload + "." + invalidSignature

	req.Header.Set(config.BypassHeader, malformedToken)

	if validator.ValidateToken(req) {
		t.Error("Should reject token with invalid signature base64")
	}
}

func TestBypassValidator_JWTAdvancedMalformation(t *testing.T) {
	config := BypassConfig{
		SecretKey:    "test-secret-key",
		TokenExpiry:  time.Minute * 5,
		BypassHeader: "X-RateLimit-Bypass",
	}

	validator := NewBypassValidator(config)

	testCases := []struct {
		name        string
		tokenParts  []string
		description string
	}{
		{
			"Valid parts count but invalid payload base64",
			[]string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "invalid-base64-!@#", "validbase64sig"},
			"Should reject token with invalid base64 in payload",
		},
		{
			"Valid base64 but invalid JSON in payload",
			[]string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "aW52YWxpZEpTT04", "validbase64sig"},
			"Should reject token with invalid JSON in payload",
		},
		{
			"Valid JSON but expired token",
			[]string{
				"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				"eyJleHAiOjEsImlhdCI6MSwiU3ViIjoicmF0ZWxpbWl0LWJ5cGFzcyJ9", // exp: 1 (expired)
				"validbase64sig",
			},
			"Should reject expired token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			token := strings.Join(tc.tokenParts, ".")
			req.Header.Set(config.BypassHeader, token)

			if validator.ValidateToken(req) {
				t.Errorf("%s: %s", tc.name, tc.description)
			}
		})
	}
}

func TestBypassValidator_CreateJWTErrorPath(t *testing.T) {
	// This test is designed to try to trigger JSON marshal errors
	// While it's difficult with simple types, we can test the normal path thoroughly
	config := BypassConfig{
		SecretKey:   "test-secret-key",
		TokenExpiry: time.Minute * 5,
	}

	validator := NewBypassValidator(config)

	// Test normal token creation - JSON marshal errors are nearly impossible with simple types
	token, err := validator.GenerateToken()
	if err != nil {
		t.Errorf("Token generation should not fail with valid config: %v", err)
	}

	if token == "" {
		t.Error("Generated token should not be empty")
	}

	// Verify the token is valid
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-RateLimit-Bypass", token)

	if !validator.ValidateToken(req) {
		t.Error("Generated token should be valid")
	}
}
