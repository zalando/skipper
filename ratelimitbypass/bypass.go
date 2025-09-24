// Package ratelimitbypass provides JWT-based bypass functionality for rate limiting
package ratelimitbypass

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// BypassConfig holds configuration for rate limit bypass functionality
type BypassConfig struct {
	SecretKey     string
	TokenExpiry   time.Duration
	BypassHeader  string
	BypassCookie  string
	IPWhitelist   []string
}

// Claims represents JWT claims for bypass tokens
type Claims struct {
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Sub string `json:"sub"`
}

// BypassValidator validates bypass tokens and IP whitelist
type BypassValidator struct {
	config     BypassConfig
	ipNets     []*net.IPNet
	secretHash []byte
}

// NewBypassValidator creates a new bypass validator
func NewBypassValidator(config BypassConfig) *BypassValidator {
	validator := &BypassValidator{
		config:     config,
		secretHash: []byte(config.SecretKey),
	}

	// Parse IP whitelist
	for _, ipStr := range config.IPWhitelist {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Handle single IPs by adding /32 or /128
		if !strings.Contains(ipStr, "/") {
			if strings.Contains(ipStr, ":") {
				ipStr += "/128" // IPv6
			} else {
				ipStr += "/32" // IPv4
			}
		}

		_, ipNet, err := net.ParseCIDR(ipStr)
		if err != nil {
			log.Warnf("Invalid IP/CIDR in whitelist: %s", ipStr)
			continue
		}
		validator.ipNets = append(validator.ipNets, ipNet)
	}

	return validator
}

// IsWhitelisted checks if the request comes from a whitelisted IP
func (v *BypassValidator) IsWhitelisted(req *http.Request) bool {
	if len(v.ipNets) == 0 {
		return false
	}

	// Get client IP from request
	clientIP := getClientIP(req)
	if clientIP == nil {
		return false
	}

	// Check against whitelist
	for _, ipNet := range v.ipNets {
		if ipNet.Contains(clientIP) {
			return true
		}
	}

	return false
}

// ValidateToken validates a bypass token from the request
func (v *BypassValidator) ValidateToken(req *http.Request) bool {
	header := v.config.BypassHeader
	if header == "" {
		header = "X-RateLimit-Bypass" // Default header
	}

	token := req.Header.Get(header)

	// If no token in header, try to get from cookie
	if token == "" && v.config.BypassCookie != "" {
		if cookie, err := req.Cookie(v.config.BypassCookie); err == nil {
			token = cookie.Value
		}
	}

	if token == "" {
		return false
	}

	return v.validateJWT(token)
}

// ShouldBypass returns true if the request should bypass rate limiting
func (v *BypassValidator) ShouldBypass(req *http.Request) bool {
	// Check IP whitelist first
	if v.IsWhitelisted(req) {
		log.Debugf("Request bypassed due to IP whitelist: %s", getClientIP(req))
		return true
	}

	// Check bypass token
	if v.ValidateToken(req) {
		log.Debugf("Request bypassed due to valid token")
		return true
	}

	return false
}

// GenerateToken generates a new bypass token
func (v *BypassValidator) GenerateToken() (string, error) {
	now := time.Now()
	claims := Claims{
		Exp: now.Add(v.config.TokenExpiry).Unix(),
		Iat: now.Unix(),
		Sub: "ratelimit-bypass",
	}

	return v.createJWT(claims)
}

// validateJWT validates a JWT token
func (v *BypassValidator) validateJWT(tokenStr string) bool {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return false
	}

	// Decode and parse claims
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	var claims Claims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return false
	}

	// Check expiration
	now := time.Now().Unix()
	if now >= claims.Exp {
		log.Debugf("Token expired: now=%d, exp=%d", now, claims.Exp)
		return false
	}

	// Verify signature
	expectedSig := v.createSignature(parts[0] + "." + parts[1])
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	return hmac.Equal(expectedSig, actualSig)
}

// createJWT creates a new JWT token
func (v *BypassValidator) createJWT(claims Claims) (string, error) {
	// Create header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Create payload
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	// Create signature
	message := headerB64 + "." + claimsB64
	signature := v.createSignature(message)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + signatureB64, nil
}

// createSignature creates HMAC-SHA256 signature
func (v *BypassValidator) createSignature(message string) []byte {
	h := hmac.New(sha256.New, v.secretHash)
	h.Write([]byte(message))
	return h.Sum(nil)
}

// getClientIP extracts client IP from request
func getClientIP(req *http.Request) net.IP {
	// Check X-Forwarded-For header
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				return parsedIP
			}
		}
	}

	// Check X-Real-IP header
	realIP := req.Header.Get("X-Real-IP")
	if realIP != "" {
		if parsedIP := net.ParseIP(realIP); parsedIP != nil {
			return parsedIP
		}
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return nil
	}

	return net.ParseIP(host)
}

// GetConfig returns the bypass configuration
func (v *BypassValidator) GetConfig() BypassConfig {
	return v.config
}

// ParseIPWhitelist parses a comma-separated list of IPs/CIDRs
func ParseIPWhitelist(whitelist string) []string {
	if whitelist == "" {
		return nil
	}

	var ips []string
	for _, ip := range strings.Split(whitelist, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}
