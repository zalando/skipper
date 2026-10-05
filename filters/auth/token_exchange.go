package auth

import (
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

	body, status, err := f.exchangeToken(ctx, subjectToken)
	if err != nil {
		ctx.Logger().Errorf("tokenExchange: %v", err)
		ctx.Serve(&http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       http.NoBody,
		})
		return
	}

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	ctx.Serve(&http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(string(body))),
	})
}

func (f *tokenExchangeFilter) Response(filters.FilterContext) {}

// exchangeToken performs the RFC 8693 token exchange and returns the raw
// response body, HTTP status code, and any transport or protocol error.
// A non-200 status from the token endpoint is returned as an error so the
// caller can serve 502 rather than forwarding an IdP error body.
func (f *tokenExchangeFilter) exchangeToken(ctx filters.FilterContext, subjectToken string) ([]byte, int, error) {
	form := url.Values{}
	form.Set("grant_type", tokenExchangeGrantType)
	form.Set("subject_token", subjectToken)
	form.Set("subject_token_type", tokenExchangeAccessTokenType)
	form.Set("requested_token_type", tokenExchangeAccessTokenType)
	form.Set("client_id", f.clientID)
	if secret, ok := f.secretsReader.GetSecret(f.clientSecretFile); ok {
		form.Set("client_secret", string(secret))
	} else {
		return nil, 0, fmt.Errorf("failed to find client_secret")
	}

	if f.audience != "" {
		form.Set("audience", f.audience)
	}
	if f.scope != "" {
		form.Set("scope", f.scope)
	}

	req, err := http.NewRequest(http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req = req.WithContext(ctx.Request().Context())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.cli.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("token exchange endpoint returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read token exchange response: %w", err)
	}

	var te tokenExchangeResponse
	if err := json.Unmarshal(raw, &te); err != nil {
		return nil, 0, fmt.Errorf("failed to decode token exchange response: %w", err)
	}
	if te.AccessToken == "" {
		return nil, 0, fmt.Errorf("token exchange response missing access_token")
	}
	return raw, resp.StatusCode, nil
}
