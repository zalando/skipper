package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/secrets"
)

const (
	tokenExchangeSpanName        = "token-exchange"
	tokenExchangeGrantType       = "urn:ietf:params:oauth:grant-type:token-exchange"
	tokenExchangeAccessTokenType = "urn:ietf:params:oauth:token-type:access_token"
)

// TokenExchangeOptions holds static, operator-supplied configuration for the
// tokenExchange filter spec.
type TokenExchangeOptions struct {
	Timeout                     time.Duration
	MaxIdleConns                int
	Tracer                      opentracing.Tracer
	OpenTracingClientTraceByTag bool
	// SecretsReader is used to read the client secret at request time from
	// the file registered as ClientSecretFile. Supports hot-reload via
	// secrets.SecretPaths.
	SecretsReader secrets.SecretsReader
}

type tokenExchangeSpec struct {
	tokenURL         string
	clientID         string
	clientSecretFile string
	options          TokenExchangeOptions
}

type tokenExchangeFilter struct {
	cli              *net.Client
	tokenURL         string
	clientID         string
	clientSecretFile string
	secretsReader    secrets.SecretsReader
	audience         string
	scope            string
}

type tokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	Scope           string `json:"scope"`
}

// tokenExchangeErrorResponse is an RFC 6749 §5.2 error response body.
type tokenExchangeErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

var tokenExchangeClients = map[string]*net.Client{}

// NewTokenExchangeSpec returns a filters.Spec for the tokenExchange() filter.
// tokenURL is the RFC 8693 token endpoint. clientID is the static client
// identifier. clientSecretFile is the path to a file containing the client
// secret; it is read at request time via o.SecretsReader, which supports
// hot-reload. The filter always responds directly to the client: on success
// it serves the RFC 8693 JSON from the token endpoint; on error it serves the
// appropriate HTTP error status. The backend is never reached.
func NewTokenExchangeSpec(tokenURL, clientID, clientSecretFile string, o TokenExchangeOptions) filters.Spec {
	return &tokenExchangeSpec{
		tokenURL:         tokenURL,
		clientID:         clientID,
		clientSecretFile: clientSecretFile,
		options:          o,
	}
}

func (*tokenExchangeSpec) Name() string { return filters.OAuthTokenExchangeName }

// CreateFilter creates a tokenExchange filter instance. It accepts zero, one,
// or two optional string arguments:
//
//	tokenExchange()                              // no audience, no scope
//	tokenExchange("https://my-service.example") // audience only
//	tokenExchange("https://my-service.example", "read write") // audience + scope
//
// The filter always calls ctx.Serve(), never forwarding to the backend.
func (s *tokenExchangeSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}

	audience := ""
	scope := ""
	if len(sargs) >= 1 {
		audience = sargs[0]
	}
	if len(sargs) >= 2 {
		scope = sargs[1]
	}

	cli, ok := tokenExchangeClients[s.tokenURL]
	if !ok {
		maxIdle := s.options.MaxIdleConns
		if maxIdle <= 0 {
			maxIdle = defaultMaxIdleConns
		}
		tracer := s.options.Tracer
		if tracer == nil {
			tracer = opentracing.NoopTracer{}
		}
		cli = net.NewClient(net.Options{
			Timeout:                 s.options.Timeout,
			MaxIdleConnsPerHost:     maxIdle,
			Tracer:                  tracer,
			OpentracingComponentTag: "skipper",
			OpentracingSpanName:     tokenExchangeSpanName,
			OpentracingEventsByTag:  s.options.OpenTracingClientTraceByTag,
		})
		tokenExchangeClients[s.tokenURL] = cli
	}

	return &tokenExchangeFilter{
		cli:              cli,
		tokenURL:         s.tokenURL,
		clientID:         s.clientID,
		clientSecretFile: s.clientSecretFile,
		secretsReader:    s.options.SecretsReader,
		audience:         audience,
		scope:            scope,
	}, nil
}

func (f *tokenExchangeFilter) Request(ctx filters.FilterContext) {
	subjectToken, ok := getToken(ctx.Request())
	if !ok || subjectToken == "" {
		unauthorized(ctx, "", missingBearerToken, "", "")
		return
	}

	successBody, idpErrBody, status, err := f.exchangeToken(ctx, subjectToken)
	if err != nil {
		// Transport/internal failure: the caller cannot act on this — serve 502.
		ctx.Logger().Errorf("tokenExchange: %v", err)
		serveTokenExchangeError(ctx, "temporarily_unavailable", err.Error(), http.StatusBadGateway)
		return
	}

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")

	if idpErrBody != nil {
		// RFC 8693 §2.2.2: forward the IdP's RFC 6749 §5.2 error response as-is.
		ctx.Serve(&http.Response{
			StatusCode: status,
			Header:     h,
			Body:       io.NopCloser(bytes.NewReader(idpErrBody)),
		})
		return
	}

	ctx.Serve(&http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(successBody)),
	})
}

func (f *tokenExchangeFilter) Response(filters.FilterContext) {}

// serveTokenExchangeError serves an RFC 6749 §5.2 JSON error response directly
// to the client. code is an OAuth 2.0 error code (e.g. "server_error",
// "temporarily_unavailable"). description is a human-readable explanation.
func serveTokenExchangeError(ctx filters.FilterContext, code, description string, status int) {
	body, _ := json.Marshal(tokenExchangeErrorResponse{Error: code, ErrorDescription: description})
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	ctx.Serve(&http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
}

// exchangeToken performs the RFC 8693 token exchange.
//
// Returns:
//   - (successBody, nil, 200, nil) on a valid 200 response with access_token
//   - (nil, idpErrBody, status, nil) when the IdP returned non-200; idpErrBody is
//     the raw response body (RFC 6749 §5.2 JSON) to forward to the caller
//   - (nil, nil, 0, err) on transport or internal failure; caller should serve 502
func (f *tokenExchangeFilter) exchangeToken(ctx filters.FilterContext, subjectToken string) ([]byte, []byte, int, error) {
	secret, ok := f.secretsReader.GetSecret(f.clientSecretFile)
	if !ok {
		return nil, nil, 0, fmt.Errorf("failed to find client_secret")
	}

	form := url.Values{}
	form.Set("grant_type", tokenExchangeGrantType)
	form.Set("subject_token", subjectToken)
	form.Set("subject_token_type", tokenExchangeAccessTokenType)
	form.Set("requested_token_type", tokenExchangeAccessTokenType)

	if f.audience != "" {
		form.Set("audience", f.audience)
	}
	if f.scope != "" {
		form.Set("scope", f.scope)
	}

	req, err := http.NewRequest(http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, 0, err
	}
	req = req.WithContext(ctx.Request().Context())
	req.SetBasicAuth(f.clientID, string(secret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.cli.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the body so the caller can forward it as an RFC 6749 §5.2 error.
		raw, _ := io.ReadAll(resp.Body)
		return nil, raw, resp.StatusCode, nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to read token exchange response: %w", err)
	}

	var te tokenExchangeResponse
	if err := json.Unmarshal(raw, &te); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to decode token exchange response: %w", err)
	}
	if te.AccessToken == "" {
		return nil, nil, 0, fmt.Errorf("token exchange response missing access_token")
	}
	if te.IssuedTokenType == "" {
		return nil, nil, 0, fmt.Errorf("token exchange response missing issued_token_type")
	}
	if te.TokenType == "" {
		return nil, nil, 0, fmt.Errorf("token exchange response missing token_type")
	}
	return raw, nil, resp.StatusCode, nil
}
