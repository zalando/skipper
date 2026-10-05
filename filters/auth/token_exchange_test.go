package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/secrets"
)

const testTokenExchangeTimeout = 100 * time.Millisecond

func TestTokenExchangeCreateFilter(t *testing.T) {
	spec := NewTokenExchangeSpec(
		"https://identity.example.com/oauth2/token",
		"test-client-id",
		"", // no secret file needed for CreateFilter unit tests
		TokenExchangeOptions{Timeout: time.Second},
	)

	for _, tc := range []struct {
		msg     string
		args    []any
		wantErr bool
	}{
		{msg: "no args", args: nil, wantErr: false},
		{msg: "audience only", args: []any{"https://my-service.example.org"}, wantErr: false},
		{msg: "audience and scope", args: []any{"https://my-service.example.org", "read write"}, wantErr: false},
		{msg: "too many args", args: []any{"https://my-service.example.org", "read", "extra"}, wantErr: true},
		{msg: "non-string audience", args: []any{42}, wantErr: true},
		{msg: "non-string scope", args: []any{"https://my-service.example.org", 42}, wantErr: true},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			f, err := spec.CreateFilter(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got filter: %v", f)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f == nil {
				t.Fatal("expected filter, got nil")
			}
		})
	}
}

// writeSecretFile writes secret to a temp file and returns the path and a cleanup func.
func writeSecretFile(t *testing.T, secret string) (string, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "token-exchange-secret-*")
	if err != nil {
		t.Fatalf("failed to create secret file: %v", err)
	}
	if _, err := f.WriteString(secret); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }
}

func TestTokenExchange(t *testing.T) {
	const (
		testClientID     = "test-client-id"
		testClientSecret = "test-client-secret"
		testExchanged    = "exchanged-token"
	)

	secretFile, cleanSecret := writeSecretFile(t, testClientSecret)
	defer cleanSecret()

	sp := secrets.NewSecretPaths(0)
	defer sp.Close()
	if err := sp.Add(secretFile); err != nil {
		t.Fatalf("failed to add secret file to SecretPaths: %v", err)
	}

	for _, tc := range []struct {
		msg             string
		incomingAuth    string
		filterArgs      []any
		tokenEndpointFn func(w http.ResponseWriter, r *http.Request)
		expectedStatus  int
		// bodyContains is a substring expected in the response body on success.
		bodyContains string
	}{
		{
			msg:          "missing Authorization header returns 401",
			incomingAuth: "",
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				// must not be reached
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			msg:          "non-Bearer Authorization returns 401",
			incomingAuth: "Basic dXNlcjpwYXNz",
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			msg:          "successful exchange serves RFC 8693 JSON to client",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, r *http.Request) {
				// RFC 6749 §2.3.1: client credentials via HTTP Basic auth
				clientID, clientSecret, ok := r.BasicAuth()
				if !ok || clientID != testClientID || clientSecret != testClientSecret {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				// RFC 8693 §2.1: validate required request parameters
				if r.FormValue("grant_type") != tokenExchangeGrantType {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.FormValue("subject_token") != testToken {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.FormValue("subject_token_type") != tokenExchangeAccessTokenType {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tokenExchangeResponse{
					AccessToken:     testExchanged,
					IssuedTokenType: tokenExchangeAccessTokenType,
					TokenType:       "Bearer",
					ExpiresIn:       3600,
				})
			},
			expectedStatus: http.StatusOK,
			bodyContains:   testExchanged,
		},
		{
			// RFC 6749 §2.3.1: client_id and client_secret must NOT appear in the form body
			msg:          "client credentials are sent as Basic auth not in request body",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, r *http.Request) {
				if r.FormValue("client_id") != "" || r.FormValue("client_secret") != "" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _, ok := r.BasicAuth()
				if !ok {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tokenExchangeResponse{
					AccessToken:     testExchanged,
					IssuedTokenType: tokenExchangeAccessTokenType,
					TokenType:       "Bearer",
					ExpiresIn:       3600,
				})
			},
			expectedStatus: http.StatusOK,
			bodyContains:   testExchanged,
		},
		{
			msg:          "audience forwarded in exchange request",
			incomingAuth: authHeaderPrefix + testToken,
			filterArgs:   []any{"https://my-service.example.org"},
			tokenEndpointFn: func(w http.ResponseWriter, r *http.Request) {
				if r.FormValue("audience") != "https://my-service.example.org" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tokenExchangeResponse{
					AccessToken:     testExchanged,
					IssuedTokenType: tokenExchangeAccessTokenType,
					TokenType:       "Bearer",
					ExpiresIn:       3600,
				})
			},
			expectedStatus: http.StatusOK,
			bodyContains:   testExchanged,
		},
		{
			msg:          "scope forwarded in exchange request",
			incomingAuth: authHeaderPrefix + testToken,
			filterArgs:   []any{"https://my-service.example.org", "read write"},
			tokenEndpointFn: func(w http.ResponseWriter, r *http.Request) {
				if r.FormValue("scope") != "read write" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tokenExchangeResponse{
					AccessToken:     testExchanged,
					IssuedTokenType: tokenExchangeAccessTokenType,
					TokenType:       "Bearer",
					ExpiresIn:       3600,
				})
			},
			expectedStatus: http.StatusOK,
			bodyContains:   testExchanged,
		},
		{
			// RFC 8693 §2.2.2: token endpoint error → forward the IdP's RFC 6749 §5.2 response
			msg:          "token endpoint returns 400 with RFC 6749 body forwards status and body",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
					Error:            "invalid_grant",
					ErrorDescription: "subject token is invalid",
				})
			},
			expectedStatus: http.StatusBadRequest,
			bodyContains:   "invalid_grant",
		},
		{
			msg:          "token endpoint returns 401 with RFC 6749 body forwards status and body",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
					Error:            "invalid_client",
					ErrorDescription: "client authentication failed",
				})
			},
			expectedStatus: http.StatusUnauthorized,
			bodyContains:   "invalid_client",
		},
		{
			msg:          "token endpoint returns 500 with RFC 6749 body forwards status and body",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(tokenExchangeErrorResponse{
					Error: "server_error",
				})
			},
			expectedStatus: http.StatusInternalServerError,
			bodyContains:   "server_error",
		},
		{
			// Empty non-200 body is forwarded with the IdP's status code.
			msg:          "token endpoint returns 400 with no body forwards 400",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			msg:          "token endpoint returns invalid JSON results in 502",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("not-json{"))
			},
			expectedStatus: http.StatusBadGateway,
		},
		{
			// RFC 8693 §2.2.1: access_token is required in a successful response
			msg:          "token endpoint returns empty access_token results in 502",
			incomingAuth: authHeaderPrefix + testToken,
			tokenEndpointFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tokenExchangeResponse{
					IssuedTokenType: tokenExchangeAccessTokenType,
					TokenType:       "Bearer",
				})
			},
			expectedStatus: http.StatusBadGateway,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(tc.tokenEndpointFn))
			defer tokenEndpoint.Close()

			spec := NewTokenExchangeSpec(
				tokenEndpoint.URL,
				testClientID,
				secretFile,
				TokenExchangeOptions{Timeout: testTokenExchangeTimeout, SecretsReader: sp},
			)

			fr := make(filters.Registry)
			fr.Register(spec)

			// The filter always serves directly; the backend is never reached.
			// Use a backend that signals if it is unexpectedly called.
			unexpectedBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("backend must not be reached: filter should have served directly")
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer unexpectedBackend.Close()

			route := &eskip.Route{
				Filters: []*eskip.Filter{{Name: spec.Name(), Args: tc.filterArgs}},
				Backend: unexpectedBackend.URL,
			}
			proxy := proxytest.New(fr, route)
			defer proxy.Close()

			req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			if tc.incomingAuth != "" {
				req.Header.Set(authHeaderName, tc.incomingAuth)
			}

			resp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("status: want %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if tc.bodyContains != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read response body: %v", err)
				}
				if !strings.Contains(string(body), tc.bodyContains) {
					t.Errorf("body: want substring %q, got %q", tc.bodyContains, string(body))
				}
			}
		})
	}
}

func TestTokenExchangeTimeout(t *testing.T) {
	const d = 50 * time.Millisecond

	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(4 * d)
		w.WriteHeader(http.StatusOK)
	}))
	defer tokenEndpoint.Close()

	// The filter serves directly; this backend must never be reached.
	unexpectedBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("backend must not be reached on timeout")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unexpectedBackend.Close()

	secretFile, cleanSecret := writeSecretFile(t, "csecret")
	defer cleanSecret()
	sp := secrets.NewSecretPaths(0)
	defer sp.Close()
	if err := sp.Add(secretFile); err != nil {
		t.Fatalf("failed to add secret file: %v", err)
	}

	spec := NewTokenExchangeSpec(tokenEndpoint.URL, "cid", secretFile, TokenExchangeOptions{Timeout: d, SecretsReader: sp})
	fr := make(filters.Registry)
	fr.Register(spec)

	route := &eskip.Route{
		Filters: []*eskip.Filter{{Name: spec.Name(), Args: nil}},
		Backend: unexpectedBackend.URL,
	}
	proxy := proxytest.New(fr, route)
	defer proxy.Close()

	req, err := http.NewRequest(http.MethodGet, proxy.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set(authHeaderName, authHeaderPrefix+testToken)

	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 on timeout, got %d", resp.StatusCode)
	}
}
