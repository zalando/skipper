package auth

import (
	"net/http"
	"net/url"

	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

// GrantCallbackName is the filter name
// Deprecated, use filters.GrantCallbackName instead
const GrantCallbackName = filters.GrantCallbackName

type grantCallbackSpec struct {
	config *OAuthConfig
}

type grantCallbackFilter struct {
	config *OAuthConfig
}

func (*grantCallbackSpec) Name() string { return filters.GrantCallbackName }

func (s *grantCallbackSpec) CreateFilter([]any) (filters.Filter, error) {
	return &grantCallbackFilter{
		config: s.config,
	}, nil
}

func (f *grantCallbackFilter) exchangeAccessToken(req *http.Request, code string) (*oauth2.Token, error) {
	authConfig, err := f.config.GetConfig(req)
	if err != nil {
		return nil, err
	}
	redirectURI, _ := f.config.RedirectURLs(req)
	ctx := providerContext(f.config)
	params := f.config.GetAuthURLParameters(redirectURI)
	return authConfig.Exchange(ctx, code, params...)
}

func (f *grantCallbackFilter) Request(ctx filters.FilterContext) {
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

	// Redirect callback request to the host of the initial request
	if initial, _ := url.Parse(state.RequestURL); initial.Host != req.Host {
		location := *req.URL
		location.Host = initial.Host
		location.Scheme = initial.Scheme

		ctx.Serve(&http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header: http.Header{
				"Location": []string{location.String()},
			},
		})
		return
	}

	token, err := f.exchangeAccessToken(req, code)
	if err != nil {
		ctx.Logger().Errorf("Failed to exchange access token: %v.", err)
		serverError(ctx)
		return
	}

	cookies, err := f.config.GrantCookieEncoder.Update(req, token)
	if err != nil {
		ctx.Logger().Errorf("Failed to create OAuth grant cookie: %v.", err)
		serverError(ctx)
		return
	}

	resp := &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": []string{state.RequestURL},
		},
	}
	for _, c := range cookies {
		resp.Header.Add("Set-Cookie", c.String())
	}
	ctx.Serve(resp)
}

func (f *grantCallbackFilter) Response(ctx filters.FilterContext) {}
