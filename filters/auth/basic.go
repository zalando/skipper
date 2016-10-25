package auth

import (
	auth "github.com/abbot/go-http-auth"
	"github.com/zalando/skipper/filters"
	"net/http"
)

const (
	Name                      = "basicAuth"
	ForceBasicAuthHeaderName  = "WWW-Authenticate"
	ForceBasicAuthHeaderValue = "Basic realm=\"User Visible Realm\""
)

type basicSpec struct{}

type basic struct {
	authenticator *auth.BasicAuth
}

func NewBasicAuth() *basicSpec {
	return &basicSpec{}
}

//We do not touch response at all
func (a *basic) Response(filters.FilterContext) {}

// check basic auth
func (a *basic) Request(ctx filters.FilterContext) {
	username := a.authenticator.CheckAuth(ctx.Request())

	if username == "" {
		header := http.Header{}
		header.Set(ForceBasicAuthHeaderName, ForceBasicAuthHeaderValue)

		ctx.Serve(&http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     header,
		})
	}
}

// Creates out basicAuth Filter
// The first params specifies the used htpasswd file
func (spec *basicSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	configFile, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	htpasswd := auth.HtpasswdFileProvider(configFile)
	authenticator := auth.NewBasicAuthenticator("Basic Realm", htpasswd)

	return &basic{authenticator: authenticator}, nil
}

func (spec *basicSpec) Name() string { return Name }
