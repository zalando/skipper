package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

type OAuthConfig struct {
	initialized bool
	initErr     error
	flowState   *flowState

	// TokeninfoURL is the URL of the service to validate OAuth2 tokens.
	TokeninfoURL string

	// Secrets is a secret registry to access secret keys used for encrypting
	// auth flow state and auth cookies.
	Secrets *secrets.Registry

	// SecretFile contains the filename with the encryption key for the authentication
	// cookie and grant flow state stored in Secrets.
	SecretFile string

	// AuthURL, the url to redirect the requests to when login is required.
	AuthURL string

	// TokenURL, the url where the access code should be exchanged for the
	// access token.
	TokenURL string

	// CallbackPath contains the path where the callback requests with the
	// authorization code should be redirected to.
	CallbackPath string

	// ClientID, the OAuth2 client id of the current service, used to exchange
	// the access code. Must be set if ClientIDFile is not provided.
	ClientID string

	// ClientSecret, the secret associated with the ClientID, used to exchange
	// the access code. Must be set if ClientSecretFile is not provided.
	ClientSecret string

	// ClientIDFile, the path to the file containing the OAuth2 client id of
	// the current service, used to exchange the access code. Must be set if
	// ClientID is not provided.
	ClientIDFile string

	// ClientSecretFile, the path to the file containing the secret associated
	// with the ClientID, used to exchange the access code. Must be set if
	// ClientSecret is not provided.
	ClientSecretFile string

	// SecretsProvider is used to read client ID and secret values from the
	// file system. Supports secret rotation.
	SecretsProvider secrets.SecretsProvider

	// TokeninfoClient, optional. When set, it will be used for the
	// authorization requests to TokeninfoURL. When not set, a new default
	// client is created.
	TokeninfoClient *authClient

	// AuthClient, optional. When set, it will be used for the
	// access code exchange requests to TokenURL. When not set, a new default
	// client is created.
	AuthClient *net.Client

	// DisableRefresh prevents refreshing the token.
	DisableRefresh bool

	// AuthURLParameters, optional. Extra URL parameters to add when calling
	// the OAuth2 authorize or token endpoints.
	AuthURLParameters map[string]string

	// AccessTokenHeaderName, optional. When set, the access token will be set
	// on the request to a header with this name.
	AccessTokenHeaderName string

	// TokeninfoSubjectKey, optional. When set, it is used to look up the subject
	// ID in the tokeninfo map received from a tokeninfo endpoint request.
	TokeninfoSubjectKey string

	// TokenCookieName, optional. The name of the cookie used to store the
	// encrypted access token after a successful token exchange.
	TokenCookieName string

	// ConnectionTimeout used for tokeninfo endpoint.
	ConnectionTimeout time.Duration

	// MaxIdleConnectionsPerHost used for tokeninfo endpoint.
	MaxIdleConnectionsPerHost int

	// Tracer used for tokeninfo endpoint.
	Tracer opentracing.Tracer
}

var (
	ErrMissingClientID        = errors.New("missing client ID")
	ErrMissingClientSecret    = errors.New("missing client secret")
	ErrMissingSecretsRegistry = errors.New("missing secrets registry")
	ErrMissingSecretFile      = errors.New("missing secret file")
	ErrMissingTokeninfoURL    = errors.New("missing tokeninfo URL")
	ErrMissingProviderURLs    = errors.New("missing provider URLs")
)

func (c *OAuthConfig) init() error {
	if c.initialized {
		return c.initErr
	}

	if c.TokeninfoURL == "" {
		c.initErr = ErrMissingTokeninfoURL
		return c.initErr
	}

	if c.AuthURL == "" || c.TokenURL == "" {
		c.initErr = ErrMissingProviderURLs
		return c.initErr
	}

	if c.Secrets == nil {
		c.initErr = ErrMissingSecretsRegistry
		return c.initErr
	}

	if c.SecretFile == "" {
		c.initErr = ErrMissingSecretFile
		return c.initErr
	}

	if c.ClientID == "" && c.ClientIDFile == "" {
		c.initErr = ErrMissingClientID
		return c.initErr
	}

	if c.ClientSecret == "" && c.ClientSecretFile == "" {
		c.initErr = ErrMissingClientSecret
		return c.initErr
	}

	if c.CallbackPath == "" {
		c.CallbackPath = defaultCallbackPath
	}

	if c.TokenCookieName == "" {
		c.TokenCookieName = defaultTokenCookieName
	}

	if c.TokeninfoClient == nil {
		client, err := newAuthClient(
			c.TokeninfoURL,
			"granttokeninfo",
			c.ConnectionTimeout,
			c.MaxIdleConnectionsPerHost,
			c.Tracer,
		)
		if err != nil {
			return err
		}
		c.TokeninfoClient = client
	}

	if c.AuthClient == nil {
		c.AuthClient = net.NewClient(net.Options{})
	}

	c.flowState = newFlowState(c.Secrets, c.SecretFile)

	if c.ClientIDFile != "" {
		c.SecretsProvider.Add(c.ClientIDFile)
	}

	if c.ClientSecretFile != "" {
		c.SecretsProvider.Add(c.ClientSecretFile)
	}

	c.initialized = true
	return nil
}

func (c *OAuthConfig) NewGrant() (filters.Spec, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	return &grantSpec{config: *c}, nil
}

func (c *OAuthConfig) NewGrantCallback() (filters.Spec, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	return &grantCallbackSpec{config: *c}, nil
}

func (c *OAuthConfig) NewGrantClaimsQuery() (filters.Spec, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	return &grantClaimsQuerySpec{
		oidcSpec: oidcIntrospectionSpec{
			typ: checkOIDCQueryClaims,
		},
	}, nil
}

func (c *OAuthConfig) NewGrantPreprocessor() (routing.PreProcessor, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	return grantPrep{config: *c}, nil
}

func (c *OAuthConfig) GetConfig() *oauth2.Config {
	return &oauth2.Config{
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURL,
			TokenURL: c.TokenURL,
		},
		ClientID:     c.GetClientID(),
		ClientSecret: c.GetClientSecret(),
	}
}

func (c *OAuthConfig) GetAuthURLParameters(redirectURI string) []oauth2.AuthCodeOption {
	params := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("redirect_uri", redirectURI)}

	if c.AuthURLParameters != nil {
		for k, v := range c.AuthURLParameters {
			params = append(params, oauth2.SetAuthURLParam(k, v))
		}
	}

	return params
}

func (c *OAuthConfig) GetClientID() string {
	if c.ClientID != "" {
		return c.ClientID
	}

	if id, exists := c.SecretsProvider.GetSecret(c.ClientIDFile); exists {
		return string(id)
	}

	return ""
}

func (c *OAuthConfig) GetClientSecret() string {
	if c.ClientSecret != "" {
		return c.ClientSecret
	}

	if secret, exists := c.SecretsProvider.GetSecret(c.ClientSecretFile); exists {
		return string(secret)
	}

	return ""
}

// RedirectURLs constructs the redirect URI based on the request and the
// configured CallbackPath.
func (c OAuthConfig) RedirectURLs(req *http.Request) (redirect, original string) {
	u := *req.URL

	if fp := req.Header.Get("X-Forwarded-Proto"); fp != "" {
		u.Scheme = fp
	} else if req.TLS != nil {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	if fh := req.Header.Get("X-Forwarded-Host"); fh != "" {
		u.Host = fh
	} else {
		u.Host = req.Host
	}

	original = u.String()

	u.Path = c.CallbackPath
	u.RawQuery = ""
	redirect = u.String()
	return
}
