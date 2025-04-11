package auth

import (
	"fmt"
	"net/http"
	"os"

	auth "github.com/abbot/go-http-auth"
	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.BasicAuthName instead
	Name = filters.BasicAuthName

	ForceBasicAuthHeaderName  = "WWW-Authenticate"
	ForceBasicAuthHeaderValue = "Basic realm="
	DefaultRealmName          = "Basic Realm"
)

type basicSpec struct{}

type basic struct {
	authenticator   *auth.BasicAuth
	realmDefinition string
}

func NewBasicAuth() *basicSpec {
	return &basicSpec{}
}

// We do not touch response at all
func (a *basic) Response(filters.FilterContext) {}

// check basic auth
func (a *basic) Request(ctx filters.FilterContext) {
	username := a.authenticator.CheckAuth(ctx.Request())

	if username == "" {
		header := http.Header{}
		header.Set(ForceBasicAuthHeaderName, a.realmDefinition)

		ctx.Serve(&http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     header,
		})
	}
}

// Creates out basicAuth Filter
// The first params specifies the used htpasswd file
// The second is optional and defines the realm name
func (spec *basicSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	configFile, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	realmName := DefaultRealmName

	if len(config) == 2 {
		if definedName, ok := config[1].(string); ok {
			realmName = definedName
		}
	}

	if _, err := os.Stat(configFile); err != nil {
		return nil, fmt.Errorf("stat failed for %q: %w", configFile, err)
	}

	htpasswd := auth.HtpasswdFileProvider(configFile)
	authenticator := auth.NewBasicAuthenticator(realmName, htpasswd)

	return &basic{
		authenticator:   authenticator,
		realmDefinition: ForceBasicAuthHeaderValue + `"` + realmName + `"`,
	}, nil
}

func (spec *basicSpec) Name() string { return filters.BasicAuthName }
