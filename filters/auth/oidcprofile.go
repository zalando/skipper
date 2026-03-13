package auth

import (
	"compress/flate"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/annotate"
	"github.com/zalando/skipper/secrets"
)

const (
	oidcProfilePrefix      = "profile:"
	oidcProfileStateBagKey = "filter.oidcProfileDelegate"
)

// OidcProfile holds pre-configured OIDC connection parameters that can
// be referenced by oauthOidc* filters using the "profile:<name>" syntax.
//
// String fields support Go text/template syntax:
//   - {{.Request.Host}} — request hostname (prefers Host header)
//   - {{index .Annotations "key"}} — value set by a preceding annotate() filter
//
// IdpURL must be a static URL (no template expressions).
type OidcProfile struct {
	IdpURL             string `yaml:"idp-url"`
	ClientID           string `yaml:"client-id"`
	ClientSecret       string `yaml:"client-secret"`
	CallbackURL        string `yaml:"callback-url"`
	Scopes             string `yaml:"scopes"`
	AuthCodeOpts       string `yaml:"auth-code-opts"`
	UpstreamHeaders    string `yaml:"upstream-headers"`
	SubdomainsToRemove string `yaml:"subdomains-to-remove"`
	CookieName         string `yaml:"cookie-name"`
}

// Validate parses each field as a Go text/template, failing fast on syntax errors.
func (p *OidcProfile) Validate() error {
	if p.IdpURL == "" {
		return fmt.Errorf("oidc profile: IdpURL is required")
	}
	for _, s := range []string{
		p.IdpURL, p.ClientID, p.ClientSecret, p.CallbackURL,
		p.Scopes, p.AuthCodeOpts, p.UpstreamHeaders, p.SubdomainsToRemove, p.CookieName,
	} {
		if s == "" {
			continue
		}
		if _, err := template.New("").Parse(s); err != nil {
			return fmt.Errorf("oidc profile template parse error in %q: %w", s, err)
		}
	}
	return nil
}

// profileRequestData holds request-scoped data accessible as {{.Request.Field}}.
type profileRequestData struct {
	Host string
}

// profileTemplateData is the data model for OIDC profile field template resolution.
type profileTemplateData struct {
	Request     profileRequestData
	Annotations map[string]string
}

// resolveField executes a Go text/template string with the given data.
// Empty tmplStr returns "" without error.
func resolveField(tmplStr string, data profileTemplateData) (string, error) {
	if tmplStr == "" {
		return "", nil
	}
	tmpl, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("template parse error in %q: %w", tmplStr, err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error in %q: %w", tmplStr, err)
	}
	return buf.String(), nil
}

// tokenOidcProfileFilter is the per-route filter created when an oauthOidc* filter
// uses the "profile:<name>" first argument syntax. It resolves profile field templates
// at request time, caches the resulting tokenOidcFilter delegates by profile name and
// request host, and delegates all OIDC processing to them.
type tokenOidcProfileFilter struct {
	name                      string // profile name, e.g. "myprofile"
	typ                       roleCheckType
	profile                   *OidcProfile
	claims                    []string // from route args; never from profile
	provider                  *oidc.Provider
	encrypter                 secrets.Encryption
	compressor                cookieCompression
	validity                  time.Duration
	subdomainsToRemoveDefault int
	oidcOptions               OidcOptions
	spec                      *tokenOidcSpec // for resolveClientCredential

	delegates sync.Map // map[string]*tokenOidcFilter, keyed by profile name + host
}

// resolvedProfile holds the template-resolved (but not secretRef-resolved) field values.
type resolvedProfile struct {
	clientID           string
	clientSecret       string
	callbackURL        string
	scopes             string
	authCodeOpts       string
	upstreamHeaders    string
	subdomainsToRemove string
	cookieName         string
}

// cacheKey returns a stable key for the delegate cache based on profile name and
// request host. Credentials are intentionally excluded: including them would cause
// unbounded cache growth on secret rotation and is unnecessary since the profile
// name and host uniquely identify the logical delegate configuration.
func (r *resolvedProfile) cacheKey(profileName, host string) string {
	return profileName + "\x00" + host
}

// resolveAll resolves all profile template fields using request-time data.
func (f *tokenOidcProfileFilter) resolveAll(data profileTemplateData) (*resolvedProfile, error) {
	var err error
	r := &resolvedProfile{}
	if r.clientID, err = resolveField(f.profile.ClientID, data); err != nil {
		return nil, fmt.Errorf("ClientID: %w", err)
	}
	if r.clientSecret, err = resolveField(f.profile.ClientSecret, data); err != nil {
		return nil, fmt.Errorf("ClientSecret: %w", err)
	}
	if r.callbackURL, err = resolveField(f.profile.CallbackURL, data); err != nil {
		return nil, fmt.Errorf("CallbackURL: %w", err)
	}
	if r.scopes, err = resolveField(f.profile.Scopes, data); err != nil {
		return nil, fmt.Errorf("scopes: %w", err)
	}
	if r.authCodeOpts, err = resolveField(f.profile.AuthCodeOpts, data); err != nil {
		return nil, fmt.Errorf("AuthCodeOpts: %w", err)
	}
	if r.upstreamHeaders, err = resolveField(f.profile.UpstreamHeaders, data); err != nil {
		return nil, fmt.Errorf("UpstreamHeaders: %w", err)
	}
	if r.subdomainsToRemove, err = resolveField(f.profile.SubdomainsToRemove, data); err != nil {
		return nil, fmt.Errorf("SubdomainsToRemove: %w", err)
	}
	if r.cookieName, err = resolveField(f.profile.CookieName, data); err != nil {
		return nil, fmt.Errorf("CookieName: %w", err)
	}
	return r, nil
}

// buildDelegate constructs a tokenOidcFilter from template-resolved profile fields.
// secretRef resolution happens here so the actual credentials are in the delegate.
// host is the request hostname, used to derive a stable cookie name.
func (f *tokenOidcProfileFilter) buildDelegate(r *resolvedProfile, host string) (*tokenOidcFilter, error) {
	clientID, err := f.spec.resolveClientCredential(r.clientID)
	if err != nil {
		return nil, fmt.Errorf("ClientID secretRef: %w", err)
	}
	clientSecret, err := f.spec.resolveClientCredential(r.clientSecret)
	if err != nil {
		return nil, fmt.Errorf("ClientSecret secretRef: %w", err)
	}

	// Cookie name: use explicit value or derive a stable name from profile name and host.
	// Credentials are intentionally excluded so the name does not change on secret
	// rotation (which would log out all users).
	cookieName := r.cookieName
	if cookieName == "" {
		h := sha256.New()
		h.Write([]byte(f.name))
		h.Write([]byte{0})
		h.Write([]byte(host))
		cookieName = oauthOidcCookieName + fmt.Sprintf("%x", h.Sum(nil))[:8] + "-"
	}

	if r.callbackURL == "" {
		return nil, fmt.Errorf("profile CallbackURL is required")
	}
	redirectURL, err := url.Parse(r.callbackURL)
	if err != nil {
		return nil, fmt.Errorf("invalid CallbackURL %q: %w", r.callbackURL, err)
	}

	subdomainsToRemove := f.subdomainsToRemoveDefault
	if r.subdomainsToRemove != "" {
		subdomainsToRemove, err = strconv.Atoi(r.subdomainsToRemove)
		if err != nil {
			return nil, fmt.Errorf("invalid SubdomainsToRemove %q: %w", r.subdomainsToRemove, err)
		}
		if subdomainsToRemove < 0 {
			return nil, fmt.Errorf("SubdomainsToRemove cannot be negative: %d", subdomainsToRemove)
		}
	}

	scopes := []string{oidc.ScopeOpenID}
	if r.scopes != "" {
		scopes = append(scopes, strings.Split(r.scopes, " ")...)
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  r.callbackURL,
		Endpoint:     f.provider.Endpoint(),
		Scopes:       scopes,
	}
	verifier := f.provider.Verifier(&oidc.Config{ClientID: clientID})

	authCodeOptions := make([]oauth2.AuthCodeOption, 0)
	var queryParams []string
	if r.authCodeOpts != "" {
		for _, p := range strings.Split(r.authCodeOpts, " ") {
			splitP := strings.SplitN(p, "=", 2)
			if len(splitP) != 2 {
				return nil, fmt.Errorf("invalid auth code opt %q", p)
			}
			if splitP[1] == "skipper-request-query" {
				queryParams = append(queryParams, splitP[0])
			} else {
				authCodeOptions = append(authCodeOptions, oauth2.SetAuthURLParam(splitP[0], splitP[1]))
			}
		}
	}

	var upstreamHeaders map[string]string
	if r.upstreamHeaders != "" {
		upstreamHeaders = make(map[string]string)
		for _, header := range strings.Split(r.upstreamHeaders, " ") {
			k, v, found := strings.Cut(header, ":")
			if !found || k == "" || v == "" {
				return nil, fmt.Errorf("malformed upstream header %q", header)
			}
			upstreamHeaders[k] = v
		}
	}

	return &tokenOidcFilter{
		typ:                f.typ,
		config:             config,
		provider:           f.provider,
		verifier:           verifier,
		claims:             f.claims,
		validity:           f.validity,
		cookiename:         cookieName,
		redirectPath:       redirectURL.Path,
		encrypter:          f.encrypter,
		authCodeOptions:    authCodeOptions,
		queryParams:        queryParams,
		compressor:         f.compressor,
		upstreamHeaders:    upstreamHeaders,
		subdomainsToRemove: subdomainsToRemove,
		oidcOptions:        f.oidcOptions,
	}, nil
}

func (f *tokenOidcProfileFilter) internalServerError(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{StatusCode: http.StatusInternalServerError})
}

// Request resolves profile templates using request-time data, looks up or builds
// a cached tokenOidcFilter delegate, stores it in the StateBag, and delegates.
func (f *tokenOidcProfileFilter) Request(ctx filters.FilterContext) {
	annotations := annotate.GetAnnotations(ctx)
	if annotations == nil {
		annotations = map[string]string{}
	}
	data := profileTemplateData{
		Request:     profileRequestData{Host: getHost(ctx.Request())},
		Annotations: annotations,
	}

	resolved, err := f.resolveAll(data)
	if err != nil {
		ctx.Logger().Errorf("oidc profile filter: failed to resolve templates: %v", err)
		f.internalServerError(ctx)
		return
	}

	host := data.Request.Host
	key := resolved.cacheKey(f.name, host)
	if d, ok := f.delegates.Load(key); ok {
		delegate := d.(*tokenOidcFilter)
		ctx.StateBag()[oidcProfileStateBagKey] = delegate
		delegate.Request(ctx)
		return
	}

	delegate, err := f.buildDelegate(resolved, host)
	if err != nil {
		ctx.Logger().Errorf("oidc profile filter: failed to build delegate: %v", err)
		f.internalServerError(ctx)
		return
	}

	actual, _ := f.delegates.LoadOrStore(key, delegate)
	delegate = actual.(*tokenOidcFilter)
	ctx.StateBag()[oidcProfileStateBagKey] = delegate
	delegate.Request(ctx)
}

// Response delegates to the tokenOidcFilter stored in the StateBag during Request.
// The base tokenOidcFilter.Response is a no-op, but we delegate for correctness.
func (f *tokenOidcProfileFilter) Response(ctx filters.FilterContext) {
	if d, ok := ctx.StateBag()[oidcProfileStateBagKey]; ok {
		d.(*tokenOidcFilter).Response(ctx)
	}
}

// newProfileFilter creates a tokenOidcProfileFilter from a pre-discovered provider.
// Called by tokenOidcSpec.CreateFilter when the first arg starts with "profile:".
func newProfileFilter(
	name string,
	typ roleCheckType,
	profile *OidcProfile,
	claims []string,
	provider *oidc.Provider,
	encrypter secrets.Encryption,
	validity time.Duration,
	subdomainsToRemoveDefault int,
	opts OidcOptions,
	spec *tokenOidcSpec,
) *tokenOidcProfileFilter {
	return &tokenOidcProfileFilter{
		name:                      name,
		typ:                       typ,
		profile:                   profile,
		claims:                    claims,
		provider:                  provider,
		encrypter:                 encrypter,
		compressor:                newDeflatePoolCompressor(flate.BestCompression),
		validity:                  validity,
		subdomainsToRemoveDefault: subdomainsToRemoveDefault,
		oidcOptions:               opts,
		spec:                      spec,
	}
}
