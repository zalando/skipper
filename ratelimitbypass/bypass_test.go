package ratelimitbypass

import (
	"net/http"
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
