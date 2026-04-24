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
	"sync/atomic"
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

	// maxDelegateCache is the maximum number of per-host delegate entries that will
	// be cached. Once exceeded, new delegates are built but not stored so a
	// misconfigured route (e.g. no Host predicate) cannot grow memory unboundedly.
	maxDelegateCache = 1000
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

// Validate checks the profile for configuration errors.
// IdpURL must be a non-empty static URL (template expressions are not allowed).
// ClientID and CallbackURL must be non-empty. All other string fields are parsed
// as Go text/template to catch syntax errors early. Static (non-templated) values
// for AuthCodeOpts, UpstreamHeaders, and SubdomainsToRemove are also validated
// semantically so broken configs fail at route creation rather than on first request.
func (p *OidcProfile) Validate() error {
	if p.IdpURL == "" {
		return fmt.Errorf("oidc profile: IdpURL is required")
	}
	if strings.Contains(p.IdpURL, "{{") {
		return fmt.Errorf("oidc profile: IdpURL must be a static URL (no template expressions): %q", p.IdpURL)
	}
	if p.ClientID == "" {
		return fmt.Errorf("oidc profile: ClientID is required")
	}
	if p.CallbackURL == "" {
		return fmt.Errorf("oidc profile: CallbackURL is required")
	}
	for _, s := range []string{
		p.ClientID, p.ClientSecret, p.CallbackURL,
		p.Scopes, p.AuthCodeOpts, p.UpstreamHeaders, p.SubdomainsToRemove, p.CookieName,
	} {
		if s == "" {
			continue
		}
		if _, err := template.New("").Parse(s); err != nil {
			return fmt.Errorf("oidc profile template parse error in %q: %w", s, err)
		}
	}
	// For static (non-templated) fields that are also semantically validated at
	// request time, pre-validate them now to surface errors at route creation.
	if !strings.Contains(p.CallbackURL, "{{") {
		u, err := url.Parse(p.CallbackURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("oidc profile: CallbackURL must be a valid URL with scheme and host, got %q", p.CallbackURL)
		}
		if u.Path == "" {
			return fmt.Errorf("oidc profile: CallbackURL must include a path, got %q", p.CallbackURL)
		}
	}
	if p.AuthCodeOpts != "" && !strings.Contains(p.AuthCodeOpts, "{{") {
		if _, _, err := parseAuthCodeOpts(p.AuthCodeOpts); err != nil {
			return fmt.Errorf("oidc profile: invalid AuthCodeOpts %q: %w", p.AuthCodeOpts, err)
		}
	}
	if p.UpstreamHeaders != "" && !strings.Contains(p.UpstreamHeaders, "{{") {
		if _, err := parseUpstreamHeaders(p.UpstreamHeaders); err != nil {
			return fmt.Errorf("oidc profile: invalid UpstreamHeaders %q: %w", p.UpstreamHeaders, err)
		}
	}
	if p.SubdomainsToRemove != "" && !strings.Contains(p.SubdomainsToRemove, "{{") {
		n, err := strconv.Atoi(p.SubdomainsToRemove)
		if err != nil {
			return fmt.Errorf("oidc profile: SubdomainsToRemove must be an integer, got %q: %w", p.SubdomainsToRemove, err)
		}
		if n < 0 {
			return fmt.Errorf("oidc profile: SubdomainsToRemove cannot be negative: %d", n)
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

	delegates     sync.Map     // map[string]*tokenOidcFilter, keyed by cacheKey(...)
	delegateCount atomic.Int64 // approximate number of entries in delegates
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

// cacheKey returns a stable key for the delegate cache.
// All resolved fields except credentials are included so that delegates keyed
// by different scopes, callbacks, auth-code options, upstream-headers, or
// cookie names are correctly distinguished.  Credentials are excluded: the
// delegate's oauth2.Config is updated in-place when a secretRef rotation is
// detected (see Request), which avoids unbounded cache growth on rotation.
// The key is the hex-encoded SHA-256 of all contributing fields, each
// length-prefixed to prevent collisions between arbitrary field values.
func cacheKey(profileName, host string, r *resolvedProfile) string {
	h := sha256.New()
	for _, s := range []string{
		profileName, host,
		r.clientID, r.callbackURL, r.scopes,
		r.authCodeOpts, r.upstreamHeaders, r.subdomainsToRemove, r.cookieName,
	} {
		fmt.Fprintf(h, "%d\x00%s\x00", len(s), s)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
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

	// Cookie name: use explicit value or derive a stable name from auth-relevant inputs,
	// aligned with the non-profile path which hashes all sargs except CallbackURL,
	// SubdomainsToRemove, and CookieName. This prevents session cross-contamination
	// when two routes share profile+host+clientID but differ in scopes or claims.
	// clientSecret is intentionally excluded so the name does not change on rotation.
	cookieName := r.cookieName
	if cookieName == "" {
		h := sha256.New()
		h.Write([]byte(f.name))
		h.Write([]byte{0})
		h.Write([]byte(host))
		h.Write([]byte{0})
		h.Write([]byte(r.clientID))
		h.Write([]byte{0})
		h.Write([]byte(r.scopes))
		h.Write([]byte{0})
		h.Write([]byte(strings.Join(f.claims, " ")))
		h.Write([]byte{0})
		h.Write([]byte(r.authCodeOpts))
		h.Write([]byte{0})
		h.Write([]byte(r.upstreamHeaders))
		cookieName = oauthOidcCookieName + fmt.Sprintf("%x", h.Sum(nil))[:8] + "-"
	}

	if r.callbackURL == "" {
		return nil, fmt.Errorf("profile CallbackURL is required")
	}
	redirectURL, err := url.Parse(r.callbackURL)
	if err != nil {
		return nil, fmt.Errorf("invalid CallbackURL %q: %w", r.callbackURL, err)
	}
	if redirectURL.Scheme == "" || redirectURL.Host == "" || redirectURL.Path == "" {
		return nil, fmt.Errorf("CallbackURL must be an absolute URL with scheme, host, and path, got %q", r.callbackURL)
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

	authCodeOptions, queryParams, err := parseAuthCodeOpts(r.authCodeOpts)
	if err != nil {
		return nil, fmt.Errorf("AuthCodeOpts: %w", err)
	}

	upstreamHeaders, err := parseUpstreamHeaders(r.upstreamHeaders)
	if err != nil {
		return nil, fmt.Errorf("UpstreamHeaders: %w", err)
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

// refreshIfNeeded re-resolves secretRef-backed credentials and, if they differ
// from the cached delegate, rebuilds the entire delegate (config + verifier)
// and replaces the cache entry. This picks up secret rotation without requiring
// a route reload. On transient resolution errors the existing delegate is
// returned unchanged.
func (f *tokenOidcProfileFilter) refreshIfNeeded(delegate *tokenOidcFilter, resolved *resolvedProfile, host, key string) *tokenOidcFilter {
	clientID, err := f.spec.resolveClientCredential(resolved.clientID)
	if err != nil {
		return delegate
	}
	clientSecret, err := f.spec.resolveClientCredential(resolved.clientSecret)
	if err != nil {
		return delegate
	}
	if delegate.config.ClientID == clientID && delegate.config.ClientSecret == clientSecret {
		return delegate
	}
	newDelegate, err := f.buildDelegate(resolved, host)
	if err != nil {
		return delegate
	}
	f.delegates.Store(key, newDelegate)
	return newDelegate
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

	if resolved.clientID == "" {
		ctx.Logger().Errorf("oidc profile filter: resolved clientID is empty for profile %q — ensure ClientID template resolves to a non-empty value", f.name)
		f.internalServerError(ctx)
		return
	}

	host := data.Request.Host
	key := cacheKey(f.name, host, resolved)
	if d, ok := f.delegates.Load(key); ok {
		delegate := f.refreshIfNeeded(d.(*tokenOidcFilter), resolved, host, key)
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

	if f.delegateCount.Load() >= maxDelegateCache {
		ctx.Logger().Errorf("oidc profile filter: delegate cache full (%d entries), building delegate without caching for host %q", maxDelegateCache, host)
		ctx.StateBag()[oidcProfileStateBagKey] = delegate
		delegate.Request(ctx)
		return
	}

	actual, loaded := f.delegates.LoadOrStore(key, delegate)
	if !loaded {
		f.delegateCount.Add(1)
	}
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
