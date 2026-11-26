package auth

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/zalando/skipper/filters"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

type OAuthConfig struct {
	initialized              bool
	flowState                *flowState
	grantTokeninfoKeysLookup map[string]struct{}
	getClientId              func(*http.Request) (string, error)
	getClientSecret          func(*http.Request) (string, error)

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

	// RevokeTokenURL, the url where the access and revoke tokens can be
	// revoked during a logout.
	RevokeTokenURL string

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
	// the current service, used to exchange the access code.
	// Must be set if ClientID is not provided.
	// File name may contain {host} placeholder which will be replaced by the request host.
	// Requires SecretsProvider, the path (or path's directory if placeholder is present) will be added to it.
	ClientIDFile string

	// ClientSecretFile, the path to the file containing the secret associated
	// with the ClientID, used to exchange the access code.
	// Must be set if ClientSecret is not provided.
	// File name may contain {host} placeholder which will be replaced by the request host.
	// Requires SecretsProvider, the path (or path's directory if placeholder is present) will be added to it.
	ClientSecretFile string

	// SecretsProvider is used to read ClientIDFile and ClientSecretFile from the
	// file system. Supports secret rotation.
	SecretsProvider secrets.SecretsProvider

	// TokeninfoClient, optional. When set, it will be used for the
	// authorization requests to TokeninfoURL. When not set, a new default
	// client is created.
	TokeninfoClient *authClient

	// AuthClient, optional. When set, it will be used for the
	// access code exchange requests to TokenURL. When not set, a new default
	// client is created.
	AuthClient *snet.Client

	// AuthURLParameters, optional. Extra URL parameters to add when calling
	// the OAuth2 authorize or token endpoints.
	AuthURLParameters map[string]string

	// AccessTokenHeaderName, optional. When set, the access token will be set
	// on the request to a header with this name.
	AccessTokenHeaderName string

	// GrantTokeninfoKeys, optional. When not empty, keys not in this list are removed from the tokeninfo map.
	GrantTokeninfoKeys []string

	// GrantCookieEncoder, optional. Cookie encoder stores and extracts OAuth token from cookies.
	GrantCookieEncoder CookieEncoder

	// TokeninfoSubjectKey, optional. When set, it is used to look up the subject
	// ID in the tokeninfo map received from a tokeninfo endpoint request.
	TokeninfoSubjectKey string

	// TokenCookieName, optional. The name of the cookie used to store the
	// encrypted access token after a successful token exchange.
	TokenCookieName string

	// TokenCookieRemoveSubdomains sets the number of subdomains to remove from
	// the callback request hostname to obtain token cookie domain.
	// Init converts default nil to 1.
	TokenCookieRemoveSubdomains *int

	// Insecure omits Secure attribute of the token cookie and uses http scheme for callback url.
	Insecure bool

	// ConnectionTimeout used for tokeninfo, access-token and refresh-token endpoint.
	ConnectionTimeout time.Duration

	// MaxIdleConnectionsPerHost used for tokeninfo, access-token and refresh-token endpoint.
	MaxIdleConnectionsPerHost int

	// Tracer used for tokeninfo, access-token and refresh-token endpoint.
	Tracer opentracing.Tracer

	// OpenTracingClientTraceByTag instead of events use span Tags
	// to measure client connection pool actions
	OpenTracingClientTraceByTag bool
}

var (
	ErrMissingClientID        = errors.New("missing client ID")
	ErrMissingClientSecret    = errors.New("missing client secret")
	ErrMissingSecretsProvider = errors.New("missing secrets provider")
	ErrMissingSecretsRegistry = errors.New("missing secrets registry")
	ErrMissingSecretFile      = errors.New("missing secret file")
	ErrMissingTokeninfoURL    = errors.New("missing tokeninfo URL")
	ErrMissingProviderURLs    = errors.New("missing provider URLs")
)

func (c *OAuthConfig) Init() error {
	if c.initialized {
		return nil
	}

	if c.TokeninfoURL == "" {
		return ErrMissingTokeninfoURL
	}

	if c.AuthURL == "" || c.TokenURL == "" {
		return ErrMissingProviderURLs
	}

	if c.Secrets == nil {
		return ErrMissingSecretsRegistry
	}

	if c.SecretFile == "" {
		return ErrMissingSecretFile
	}

	if c.CallbackPath == "" {
		c.CallbackPath = defaultCallbackPath
	}

	if c.TokenCookieName == "" {
		c.TokenCookieName = defaultTokenCookieName
	}

	if c.TokenCookieRemoveSubdomains == nil {
		one := 1
		c.TokenCookieRemoveSubdomains = &one
	} else if *c.TokenCookieRemoveSubdomains < 0 {
		return fmt.Errorf("invalid number of cookie subdomains to remove")
	}

	if c.TokeninfoClient == nil {
		client, err := newAuthClient(
			c.TokeninfoURL,
			"granttokeninfo",
			c.ConnectionTimeout,
			c.MaxIdleConnectionsPerHost,
			c.Tracer,
			c.OpenTracingClientTraceByTag,
		)
		if err != nil {
			return err
		}
		c.TokeninfoClient = client
	}

	if c.AuthClient == nil {
		c.AuthClient = snet.NewClient(snet.Options{
			ResponseHeaderTimeout:   c.ConnectionTimeout,
			TLSHandshakeTimeout:     c.ConnectionTimeout,
			MaxIdleConnsPerHost:     c.MaxIdleConnectionsPerHost,
			Tracer:                  c.Tracer,
			OpentracingComponentTag: "skipper",
			OpentracingSpanName:     "grantauth",
			OpentracingEventsByTag:  c.OpenTracingClientTraceByTag,
		})
	}

	c.flowState = newFlowState(c.Secrets, c.SecretFile)

	if c.ClientID != "" {
		c.getClientId = func(*http.Request) (string, error) {
			return c.ClientID, nil
		}
	} else if c.ClientIDFile != "" {
		if c.SecretsProvider == nil {
			return ErrMissingSecretsProvider
		}
		if hasPlaceholders(c.ClientIDFile) {
			c.getClientId = func(req *http.Request) (string, error) {
				return c.getSecret(resolvePlaceholders(c.ClientIDFile, req))
			}
			if err := c.SecretsProvider.Add(filepath.Dir(c.ClientIDFile)); err != nil {
				return err
			}
		} else {
			c.getClientId = func(*http.Request) (string, error) {
				return c.getSecret(c.ClientIDFile)
			}
			if err := c.SecretsProvider.Add(c.ClientIDFile); err != nil {
				return err
			}
		}
	} else {
		return ErrMissingClientID
	}

	if c.ClientSecret != "" {
		c.getClientSecret = func(*http.Request) (string, error) {
			return c.ClientSecret, nil
		}
	} else if c.ClientSecretFile != "" {
		if c.SecretsProvider == nil {
			return ErrMissingSecretsProvider
		}
		if hasPlaceholders(c.ClientSecretFile) {
			c.getClientSecret = func(req *http.Request) (string, error) {
				return c.getSecret(resolvePlaceholders(c.ClientSecretFile, req))
			}
			if err := c.SecretsProvider.Add(filepath.Dir(c.ClientSecretFile)); err != nil {
				return err
			}
		} else {
			c.getClientSecret = func(*http.Request) (string, error) {
				return c.getSecret(c.ClientSecretFile)
			}
			if err := c.SecretsProvider.Add(c.ClientSecretFile); err != nil {
				return err
			}
		}
	} else {
		return ErrMissingClientSecret
	}

	if len(c.GrantTokeninfoKeys) > 0 {
		c.grantTokeninfoKeysLookup = make(map[string]struct{}, len(c.GrantTokeninfoKeys))
		for _, key := range c.GrantTokeninfoKeys {
			c.grantTokeninfoKeysLookup[key] = struct{}{}
		}
	}

	if c.GrantCookieEncoder == nil {
		encryption, err := c.Secrets.GetEncrypter(secretsRefreshInternal, c.SecretFile)
		if err != nil {
			return err
		}
		c.GrantCookieEncoder = &EncryptedCookieEncoder{
			Encryption:       encryption,
			CookieName:       c.TokenCookieName,
			RemoveSubdomains: *c.TokenCookieRemoveSubdomains,
			Insecure:         c.Insecure,
		}
	}

	c.initialized = true
	return nil
}

func (c *OAuthConfig) NewGrant() filters.Spec {
	return &grantSpec{config: c}
}

func (c *OAuthConfig) NewGrantCallback() filters.Spec {
	return &grantCallbackSpec{config: c}
}

func (c *OAuthConfig) NewGrantClaimsQuery() filters.Spec {
	return &grantClaimsQuerySpec{
		oidcSpec: oidcIntrospectionSpec{
			typ: checkOIDCQueryClaims,
		},
	}
}

func (c *OAuthConfig) NewGrantLogout() filters.Spec {
	return &grantLogoutSpec{config: c}
}

func (c *OAuthConfig) NewGrantPreprocessor() routing.PreProcessor {
	return &grantPrep{config: c}
}

func (c *OAuthConfig) GetConfig(req *http.Request) (*oauth2.Config, error) {
	var err error
	authConfig := &oauth2.Config{
		Endpoint: oauth2.Endpoint{
			AuthURL:  c.AuthURL,
			TokenURL: c.TokenURL,
		},
	}

	authConfig.ClientID, err = c.getClientId(req)
	if err != nil {
		return nil, err
	}

	authConfig.ClientSecret, err = c.getClientSecret(req)
	if err != nil {
		return nil, err
	}

	return authConfig, nil
}

func (c *OAuthConfig) getSecret(file string) (string, error) {
	if secret, ok := c.SecretsProvider.GetSecret(file); ok {
		return string(secret), nil
	} else {
		return "", fmt.Errorf("secret %s does not exist", file)
	}
}

func resolvePlaceholders(s string, r *http.Request) string {
	h, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		h = r.Host
	}
	return strings.ReplaceAll(s, "{host}", h)
}

func hasPlaceholders(s string) bool {
	return resolvePlaceholders(s, &http.Request{Host: "example.org"}) != s
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

// RedirectURLs constructs the redirect URI based on the request and the
// configured CallbackPath.
// X-Skipper-Redirect-Host header overrides the host generated in the redirect URL
func (c *OAuthConfig) RedirectURLs(req *http.Request) (redirect, original string) {
	u := *req.URL

	if c.Insecure {
		u.Scheme = "http"
	} else {
		u.Scheme = "https"
	}

	original = u.String()

	redirectBaseOverride := req.Header.Get("X-Skipper-Redirect-Base-Uri")
	if redirectBaseOverride != "" {
		u, err := url.Parse(redirectBaseOverride)
		if err == nil {
			redirect = (&url.URL{
				Scheme: u.Scheme,
				Host:   u.Host,
				Path:   c.CallbackPath,
			}).String()
			return
		}
	}

	u.Host = req.Host
	redirect = (&url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   c.CallbackPath,
	}).String()

	return
}
