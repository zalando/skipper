package auth

import (
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

const GrantCallbackName = "grantCallback"

type grantCallbackSpec struct {
	config OAuthConfig
}

type grantCallbackFilter struct {
	config OAuthConfig
}

func (grantCallbackSpec) Name() string { return GrantCallbackName }

func (s grantCallbackSpec) CreateFilter([]interface{}) (filters.Filter, error) {
	return grantCallbackFilter(s), nil
}

func (f grantCallbackFilter) exchangeAccessToken(code string, redirectURI string) (*oauth2.Token, error) {
	ctx := providerContext(f.config)
	params := f.config.GetAuthURLParameters(redirectURI)
	return f.config.GetConfig().Exchange(ctx, code, params...)
}

func (f grantCallbackFilter) loginCallback(ctx filters.FilterContext) {
	req := ctx.Request()
	q := req.URL.Query()

	code := q.Get("code")
	if code == "" {
		badRequest(ctx)
		return
	}

	queryState := q.Get("state")
	if queryState == "" {
		badRequest(ctx)
		return
	}

	state, err := f.config.flowState.extractState(queryState)
	if err != nil {
		log.Errorf("Error when extracting flow state: %v.", err)

		if err == errExpiredAuthState {
			// The login flow state expired. Instead of just returning an
			// error, restart the login process with the original request
			// URL.
			loginRedirectWithOverride(ctx, f.config, state.RequestURL)
		} else {
			serverError(ctx)
		}
		return
	}

	redirectURI, _ := f.config.RedirectURLs(req)
	token, err := f.exchangeAccessToken(code, redirectURI)
	if err != nil {
		log.Errorf("Error when exchanging access token: %v.", err)
		serverError(ctx)
		return
	}

	c, err := CreateCookie(f.config, req.Host, token)
	if err != nil {
		log.Errorf("Error while creating OAuth grant cookie: %v.", err)
		serverError(ctx)
		return
	}

	ctx.Serve(&http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location":   []string{state.RequestURL},
			"Set-Cookie": []string{c.String()},
		},
	})
}

func (f grantCallbackFilter) Request(ctx filters.FilterContext) {
	f.loginCallback(ctx)
}

func (f grantCallbackFilter) Response(ctx filters.FilterContext) {}
