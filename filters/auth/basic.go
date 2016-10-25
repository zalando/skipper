package auth

import (
	auth "github.com/abbot/go-http-auth"
	"github.com/zalando/skipper/filters"
	"net/http"
)

const (
	Name                      = "basicAuth"
	ForceBasicAuthHeaderName  = "WWW-Authenticate"
	ForceBasicAuthHeaderValue = "Basic realm="
	DefaultRealmName          = "Basic Realm"
)

type basicSpec struct{}

type basic struct {
	authenticator *auth.BasicAuth
	realmName     string
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
		header.Set(ForceBasicAuthHeaderName, ForceBasicAuthHeaderValue+"\""+a.realmName+"\"")

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

	realmName := DefaultRealmName

	if len(config) == 2 {
		definedName, ok := config[1].(string)
		if ok {
			realmName = definedName
		}
	}

	htpasswd := auth.HtpasswdFileProvider(configFile)
	authenticator := auth.NewBasicAuthenticator(realmName, htpasswd)

	return &basic{authenticator: authenticator, realmName: realmName}, nil
}

func (spec *basicSpec) Name() string { return Name }
