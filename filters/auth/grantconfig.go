package auth

import (
	"errors"

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
	oauthConfig *oauth2.Config

	// TokeninfoURL is the URL of the service to validate OAuth2 tokens.
	TokeninfoURL string

	// Secrets is a secret registry to access secret keys used for encrypting
	// auth flow state and auth cookies.
	Secrets *secrets.Registry

	// SecretFile contains the filename with the encryption key for the authentication
	// cookie and grant flow state stored in Secrets.
	SecretFile string

	// AuthURL, the url to redirect the requests to when login is require.
	AuthURL string

	// TokenURL, the url where the access code should be exchanged for the
	// access token.
	TokenURL string

	// CallbackPath contains the path where the callback requests with the
	// authorization code should be redirected to.
	CallbackPath string

	// ClientID, the OAuth2 client id of the current service, used to exchange
	// the access code.
	ClientID string

	// ClientSecret, the secret associated with the ClientID, used to exchange
	// the access code.
	ClientSecret string

	// TokeninfoClient, optional. When set, it will be used for the
	// authorization requests to TokeninfoURL. When not set, a new default
	// client is created.
	TokeninfoClient *net.Client

	// AuthClient, optional. When set, it will be used for the
	// access code exchange requests to TokenURL. When not set, a new default
	// client is created.
	AuthClient *net.Client

	// DisableRefresh prevents refreshing the token.
	DisableRefresh bool
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

	if c.ClientID == "" {
		c.initErr = ErrMissingClientID
		return c.initErr
	}

	if c.ClientSecret == "" {
		c.initErr = ErrMissingClientSecret
		return c.initErr
	}

	if c.CallbackPath == "" {
		c.CallbackPath = defaultCallbackPath
	}

	if c.TokeninfoClient == nil {
		c.TokeninfoClient = net.NewClient(net.Options{})
	}

	if c.AuthClient == nil {
		c.AuthClient = net.NewClient(net.Options{})
	}

	c.flowState = newFlowState(c.Secrets, c.SecretFile)
	c.oauthConfig = &oauth2.Config{
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURL,
			TokenURL: c.TokenURL,
		},
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
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

func (c *OAuthConfig) NewGrantPreprocessor() (routing.PreProcessor, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	return grantPrep{config: *c}, nil
}
