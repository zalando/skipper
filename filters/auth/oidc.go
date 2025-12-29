package auth

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/zalando/skipper/filters"
	snet "github.com/zalando/skipper/net"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

const (
	// Deprecated, use filters.OAuthOidcUserInfoName instead
	OidcUserInfoName = filters.OAuthOidcUserInfoName
	// Deprecated, use filters.OAuthOidcAnyClaimsName instead
	OidcAnyClaimsName = filters.OAuthOidcAnyClaimsName
	// Deprecated, use filters.OAuthOidcAllClaimsName instead
	OidcAllClaimsName = filters.OAuthOidcAllClaimsName

	oauthOidcCookieName   = "skipperOauthOidc"
	stateValidity         = 1 * time.Minute
	defaultCookieValidity = 1 * time.Hour
	oidcInfoHeader        = "Skipper-Oidc-Info"
	cookieMaxSize         = 4093 // common cookie size limit http://browsercookielimits.squawky.net/

	// Deprecated: The host of the Azure Active Directory (AAD) graph API
	azureADGraphHost = "graph.windows.net"
)

var (
	distributedClaimsClients = sync.Map{}
	microsoftGraphHost       = "graph.microsoft.com" // global for testing
)

type distributedClaims struct {
	ClaimNames   map[string]string      `json:"_claim_names"`
	ClaimSources map[string]claimSource `json:"_claim_sources"`
}

type claimSource struct {
	Endpoint    string `json:"endpoint"`
	AccessToken string `json:"access_token,omitempty"`
}

type azureGraphGroups struct {
	OdataNextLink string `json:"@odata.nextLink,omitempty"`
	Value         []struct {
		OnPremisesSamAccountName string `json:"onPremisesSamAccountName"`
		ID                       string `json:"id"`
	} `json:"value"`
}

// Filter parameter:
//
//	oauthOidc...("https://oidc-provider.example.com", "client_id", "client_secret",
//	             "http://target.example.com/subpath/callback", "email profile", "name email picture",
//	             "parameter=value", "X-Auth-Authorization:claims.email")
const (
	paramIdpURL int = iota
	paramClientID
	paramClientSecret
	paramCallbackURL
	paramScopes
	paramClaims
	paramAuthCodeOpts
	paramUpstrHeaders
	paramSubdomainsToRemove
	paramCookieName
)

type OidcOptions struct {
	MaxIdleConns           int
	CookieRemoveSubdomains *int
	CookieValidity         time.Duration
	Timeout                time.Duration
	Tracer                 opentracing.Tracer
	OidcClientId           string
	OidcClientSecret       string
}

type (
	tokenOidcSpec struct {
		typ             roleCheckType
		SecretsFile     string
		secretsRegistry secrets.EncrypterCreator
		options         OidcOptions
	}

	tokenOidcFilter struct {
		typ                roleCheckType
		config             *oauth2.Config
		provider           *oidc.Provider
		verifier           *oidc.IDTokenVerifier
		claims             []string
		validity           time.Duration
		cookiename         string
		redirectPath       string
		encrypter          secrets.Encryption
		authCodeOptions    []oauth2.AuthCodeOption
		queryParams        []string
		compressor         cookieCompression
		upstreamHeaders    map[string]string
		subdomainsToRemove int
		oidcOptions        OidcOptions
	}

	tokenContainer struct {
		OAuth2Token *oauth2.Token          `json:"oauth2token"`
		OIDCIDToken string                 `json:"oidctoken"`
		UserInfo    *oidc.UserInfo         `json:"userInfo,omitempty"`
		Subject     string                 `json:"subject"`
		Claims      map[string]interface{} `json:"claims"`
	}

	cookieCompression interface {
		compress([]byte) ([]byte, error)
		decompress([]byte) ([]byte, error)
	}
	deflatePoolCompressor struct {
		poolWriter *sync.Pool
	}
)

// NewOAuthOidcUserInfosWithOptions creates filter spec which tests user info.
func NewOAuthOidcUserInfosWithOptions(secretsFile string, secretsRegistry secrets.EncrypterCreator, o OidcOptions) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: secretsFile, secretsRegistry: secretsRegistry, options: o}
}

// Deprecated: use NewOAuthOidcUserInfosWithOptions instead.
func NewOAuthOidcUserInfos(secretsFile string, secretsRegistry secrets.EncrypterCreator) filters.Spec {
	return NewOAuthOidcUserInfosWithOptions(secretsFile, secretsRegistry, OidcOptions{})
}

// NewOAuthOidcAnyClaimsWithOptions creates a filter spec which verifies that the token
// has one of the claims specified
func NewOAuthOidcAnyClaimsWithOptions(secretsFile string, secretsRegistry secrets.EncrypterCreator, o OidcOptions) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: secretsFile, secretsRegistry: secretsRegistry, options: o}
}

// Deprecated: use NewOAuthOidcAnyClaimsWithOptions instead.
func NewOAuthOidcAnyClaims(secretsFile string, secretsRegistry secrets.EncrypterCreator) filters.Spec {
	return NewOAuthOidcAnyClaimsWithOptions(secretsFile, secretsRegistry, OidcOptions{})
}

// NewOAuthOidcAllClaimsWithOptions creates a filter spec which verifies that the token
// has all the claims specified
func NewOAuthOidcAllClaimsWithOptions(secretsFile string, secretsRegistry secrets.EncrypterCreator, o OidcOptions) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: secretsFile, secretsRegistry: secretsRegistry, options: o}
}

// Deprecated: use NewOAuthOidcAllClaimsWithOptions instead.
func NewOAuthOidcAllClaims(secretsFile string, secretsRegistry secrets.EncrypterCreator) filters.Spec {
	return NewOAuthOidcAllClaimsWithOptions(secretsFile, secretsRegistry, OidcOptions{})
}

// CreateFilter creates an OpenID Connect authorization filter.
//
// first arg: a provider, for example "https://accounts.google.com",
// which has the path /.well-known/openid-configuration
//
// Example:
//
//	oauthOidcAllClaims("https://accounts.identity-provider.com", "some-client-id", "some-client-secret",
//	"http://callback.com/auth/provider/callback", "scope1 scope2", "claim1 claim2", "<optional>", "<optional>", "<optional>") -> "https://internal.example.org";
func (s *tokenOidcSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) <= paramClaims {
		return nil, filters.ErrInvalidFilterParameters
	}

	issuerURL, err := url.Parse(sargs[paramIdpURL])
	if err != nil {
		log.Errorf("Failed to parse url %s: %v.", sargs[paramIdpURL], err)
		return nil, filters.ErrInvalidFilterParameters
	}

	provider, err := oidc.NewProvider(context.Background(), issuerURL.String())
	if err != nil {
		log.Errorf("Failed to create new provider %s: %v.", issuerURL, err)
		return nil, filters.ErrInvalidFilterParameters
	}

	var cookieName string
	if len(sargs) > paramCookieName && sargs[paramCookieName] != "" {
		cookieName = sargs[paramCookieName]
	} else {
		h := sha256.New()
		for i, s := range sargs {
			// CallbackURL not taken into account for cookie hashing for additional sub path ingresses
			if i == paramCallbackURL {
				continue
			}
			// SubdomainsToRemove not taken into account for cookie hashing for additional sub-domain ingresses
			if i == paramSubdomainsToRemove {
				continue
			}
			h.Write([]byte(s))
		}
		byteSlice := h.Sum(nil)
		sargsHash := fmt.Sprintf("%x", byteSlice)[:8]
		cookieName = oauthOidcCookieName + sargsHash + "-"
	}
	log.Debugf("Cookie Name: %s", cookieName)

	redirectURL, err := url.Parse(sargs[paramCallbackURL])
	if err != nil || sargs[paramCallbackURL] == "" {
		return nil, fmt.Errorf("invalid redirect url '%s': %w", sargs[paramCallbackURL], err)
	}

	encrypter, err := s.secretsRegistry.GetEncrypter(1*time.Minute, s.SecretsFile)
	if err != nil {
		return nil, err
	}

	subdomainsToRemove := 1
	if s.options.CookieRemoveSubdomains != nil {
		subdomainsToRemove = *s.options.CookieRemoveSubdomains
	}
	if len(sargs) > paramSubdomainsToRemove && sargs[paramSubdomainsToRemove] != "" {
		subdomainsToRemove, err = strconv.Atoi(sargs[paramSubdomainsToRemove])
		if err != nil {
			return nil, err
		}
	}
	if subdomainsToRemove < 0 {
		return nil, fmt.Errorf("domain level cannot be negative '%d'", subdomainsToRemove)
	}

	validity := s.options.CookieValidity
	if validity == 0 {
		validity = defaultCookieValidity
	}

	oidcClientId := sargs[paramClientID]
	if oidcClientId == "" {
		oidcClientId = s.options.OidcClientId
	}

	oidcClientSecret := sargs[paramClientSecret]
	if oidcClientSecret == "" {
		oidcClientSecret = s.options.OidcClientSecret
	}

	f := &tokenOidcFilter{
		typ:          s.typ,
		redirectPath: redirectURL.Path,
		config: &oauth2.Config{
			ClientID:     oidcClientId,
			ClientSecret: oidcClientSecret,
			RedirectURL:  sargs[paramCallbackURL], // self endpoint
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID}, // mandatory scope by spec
		},
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: oidcClientId,
		}),
		validity:           validity,
		cookiename:         cookieName,
		encrypter:          encrypter,
		compressor:         newDeflatePoolCompressor(flate.BestCompression),
		subdomainsToRemove: subdomainsToRemove,
		oidcOptions:        s.options,
	}

	// user defined scopes
	scopes := strings.Split(sargs[paramScopes], " ")
	if len(sargs[paramScopes]) == 0 {
		scopes = []string{}
	}
	// scopes are only used to request claims to be in the IDtoken requested from auth server
	// https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims
	f.config.Scopes = append(f.config.Scopes, scopes...)
	// user defined claims to check for authnz
	if len(sargs[paramClaims]) > 0 {
		f.claims = strings.Split(sargs[paramClaims], " ")
	}

	f.authCodeOptions = make([]oauth2.AuthCodeOption, 0)
	if len(sargs) > paramAuthCodeOpts && sargs[paramAuthCodeOpts] != "" {
		extraParameters := strings.Split(sargs[paramAuthCodeOpts], " ")

		for _, p := range extraParameters {
			splitP := strings.Split(p, "=")
			log.Debug(splitP)
			if len(splitP) != 2 {
				return nil, filters.ErrInvalidFilterParameters
			}
			if splitP[1] == "skipper-request-query" {
				f.queryParams = append(f.queryParams, splitP[0])
			} else {
				f.authCodeOptions = append(f.authCodeOptions, oauth2.SetAuthURLParam(splitP[0], splitP[1]))
			}
		}
	}
	log.Debugf("Auth Code Options: %v", f.authCodeOptions)

	// inject additional headers from the access token for upstream applications
	if len(sargs) > paramUpstrHeaders && sargs[paramUpstrHeaders] != "" {
		f.upstreamHeaders = make(map[string]string)

		for _, header := range strings.Split(sargs[paramUpstrHeaders], " ") {
			k, v, found := strings.Cut(header, ":")
			if !found || k == "" || v == "" {
				return nil, fmt.Errorf("%w: malformed filter for upstream headers %s", filters.ErrInvalidFilterParameters, header)
			}
			f.upstreamHeaders[k] = v
		}
		log.Debugf("Upstream Headers: %v", f.upstreamHeaders)
	}

	return f, nil
}

func (s *tokenOidcSpec) Name() string {
	switch s.typ {
	case checkOIDCUserInfo:
		return filters.OAuthOidcUserInfoName
	case checkOIDCAnyClaims:
		return filters.OAuthOidcAnyClaimsName
	case checkOIDCAllClaims:
		return filters.OAuthOidcAllClaimsName
	}
	return AuthUnknown
}

func (f *tokenOidcFilter) validateAnyClaims(h map[string]interface{}) bool {
	if len(f.claims) == 0 {
		return true
	}
	if len(h) == 0 {
		return false
	}

	for _, c := range f.claims {
		if _, ok := h[c]; ok {
			return true
		}
	}
	return false
}

func (f *tokenOidcFilter) validateAllClaims(h map[string]interface{}) bool {
	l := len(f.claims)
	if l == 0 {
		return true
	}
	if len(h) < l {
		return false
	}

	for _, c := range f.claims {
		if _, ok := h[c]; !ok {
			return false
		}
	}
	return true
}

type OauthState struct {
	Validity    int64  `json:"validity"`
	Nonce       string `json:"nonce"`
	RedirectUrl string `json:"redirectUrl"`
}

func createState(nonce []byte, redirectUrl string) ([]byte, error) {
	state := &OauthState{
		Validity:    time.Now().Add(stateValidity).Unix(),
		Nonce:       fmt.Sprintf("%x", nonce),
		RedirectUrl: redirectUrl,
	}
	return json.Marshal(state)
}

func extractState(encState []byte) (*OauthState, error) {
	var state OauthState
	err := json.Unmarshal(encState, &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (f *tokenOidcFilter) internalServerError(ctx filters.FilterContext) {
	rsp := &http.Response{
		StatusCode: http.StatusInternalServerError,
	}
	ctx.Serve(rsp)
}

// https://openid.net/specs/openid-connect-core-1_0.html#CodeFlowSteps
// 1. Client prepares an Authentication Request containing the desired request parameters.
// 2. Client sends the request to the Authorization Server.
func (f *tokenOidcFilter) doOauthRedirect(ctx filters.FilterContext, cookies []*http.Cookie) {
	nonce, err := f.encrypter.CreateNonce()
	if err != nil {
		ctx.Logger().Errorf("Failed to create nonce: %v.", err)
		f.internalServerError(ctx)
		return
	}

	redirectUrl := ctx.Request().URL.String()
	statePlain, err := createState(nonce, redirectUrl)
	if err != nil {
		ctx.Logger().Errorf("Failed to create oauth2 state: %v.", err)
		f.internalServerError(ctx)
		return
	}
	stateEnc, err := f.encrypter.Encrypt(statePlain)
	if err != nil {
		ctx.Logger().Errorf("Failed to encrypt data block: %v.", err)
		f.internalServerError(ctx)
		return
	}

	opts := f.authCodeOptions
	if f.queryParams != nil {
		opts = make([]oauth2.AuthCodeOption, len(f.authCodeOptions), len(f.authCodeOptions)+len(f.queryParams))
		copy(opts, f.authCodeOptions)
		for _, p := range f.queryParams {
			if v := ctx.Request().URL.Query().Get(p); v != "" {
				opts = append(opts, oauth2.SetAuthURLParam(p, v))
			}
		}
	}

	oauth2URL := f.config.AuthCodeURL(fmt.Sprintf("%x", stateEnc), opts...)
	rsp := &http.Response{
		Header: http.Header{
			"Location": []string{oauth2URL},
		},
		StatusCode: http.StatusTemporaryRedirect,
		Status:     "Moved Temporarily",
	}
	for _, cookie := range cookies {
		rsp.Header.Add("Set-Cookie", cookie.String())
	}
	ctx.Logger().Debugf("serve redirect: plaintextState:%s to Location: %s", statePlain, rsp.Header.Get("Location"))
	ctx.Serve(rsp)
}

func (f *tokenOidcFilter) Response(filters.FilterContext) {}

func extractDomainFromHost(host string, subdomainsToRemove int) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	ip := net.ParseIP(h)
	if ip != nil {
		return ip.String()
	}
	if subdomainsToRemove == 0 {
		return h
	}
	subDomains := strings.Split(h, ".")
	if len(subDomains)-subdomainsToRemove < 2 {
		return h
	}
	return strings.Join(subDomains[subdomainsToRemove:], ".")
}

func getHost(request *http.Request) string {
	if h := request.Header.Get("host"); h != "" {
		return h
	} else {
		return request.Host
	}
}

func (f *tokenOidcFilter) createOidcCookie(ctx filters.FilterContext, name string, value string, maxAge int) (cookie *http.Cookie) {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		MaxAge:   maxAge,
		Domain:   extractDomainFromHost(getHost(ctx.Request()), f.subdomainsToRemove),
	}
}

func (f *tokenOidcFilter) deleteOidcCookie(ctx filters.FilterContext, name string) (cookie *http.Cookie) {
	return f.createOidcCookie(ctx, name, "", -1)
}

func chunkCookie(cookie *http.Cookie) (cookies []*http.Cookie) {
	// We need to dereference the cookie to avoid modifying the original cookie.
	cookieCopy := *cookie

	for index := 'a'; index <= 'z'; index++ {
		cookieSize := len(cookieCopy.String())
		if cookieSize < cookieMaxSize {
			cookieCopy.Name += string(index)
			return append(cookies, &cookieCopy)
		}

		newCookie := cookieCopy
		newCookie.Name += string(index)
		// non-deterministic approach support signature changes
		cut := len(cookieCopy.Value) - (cookieSize - cookieMaxSize) - 1
		newCookie.Value, cookieCopy.Value = cookieCopy.Value[:cut], cookieCopy.Value[cut:]
		cookies = append(cookies, &newCookie)
	}
	log.Error("unsupported amount of chunked cookies")
	return
}

func mergerCookies(cookies []*http.Cookie) *http.Cookie {
	if len(cookies) == 0 {
		return nil
	}
	cookie := *(cookies[0])
	cookie.Name = cookie.Name[:len(cookie.Name)-1]
	cookie.Value = ""
	// potentially shuffled
	sort.Slice(cookies, func(i, j int) bool {
		return cookies[i].Name < cookies[j].Name
	})
	for _, ck := range cookies {
		cookie.Value += ck.Value
	}
	return &cookie
}

func (f *tokenOidcFilter) doDownstreamRedirect(ctx filters.FilterContext, oidcState []byte, maxAge time.Duration, redirectUrl string) {
	ctx.Logger().Debugf("Doing Downstream Redirect to :%s", redirectUrl)
	r := &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": {redirectUrl},
		},
	}
	oidcCookies := chunkCookie(
		f.createOidcCookie(
			ctx,
			f.cookiename,
			base64.StdEncoding.EncodeToString(oidcState),
			int(maxAge.Seconds()),
		),
	)
	for _, cookie := range oidcCookies {
		r.Header.Add("Set-Cookie", cookie.String())
	}
	ctx.Serve(r)
}

func (f *tokenOidcFilter) validateCookie(cookie *http.Cookie) ([]byte, bool) {
	if cookie == nil {
		log.Debugf("Cookie is nil")
		return nil, false
	}
	log.Debugf("validate cookie name: %s", f.cookiename)
	decodedCookie, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		log.Debugf("Base64 decoding the cookie failed: %v", err)
		return nil, false
	}
	decryptedCookie, err := f.encrypter.Decrypt(decodedCookie)
	if err != nil {
		log.Debugf("Decrypting the cookie failed: %v", err)
		return nil, false
	}
	decompressedCookie, err := f.compressor.decompress(decryptedCookie)
	if err != nil {
		log.Error(err)
		return nil, false
	}
	return decompressedCookie, true
}

// https://openid.net/specs/openid-connect-core-1_0.html#CodeFlowSteps
// 5. Authorization Server sends the End-User back to the Client with an Authorization Code.
func (f *tokenOidcFilter) callbackEndpoint(ctx filters.FilterContext) {
	var (
		claimsMap   map[string]interface{}
		oauth2Token *oauth2.Token
		data        []byte
		resp        tokenContainer
		sub         string
		userInfo    *oidc.UserInfo
		oidcIDToken string
	)

	r := ctx.Request()
	oauthState, err := f.getCallbackState(ctx)
	if err != nil {
		if _, ok := err.(*requestError); !ok {
			ctx.Logger().Errorf("Error while retrieving callback state: %v.", err)
		}

		unauthorized(
			ctx,
			"",
			invalidToken,
			r.Host,
			fmt.Sprintf("Failed to get state from callback: %v.", err),
		)

		return
	}

	oauth2Token, err = f.getTokenWithExchange(oauthState, ctx)
	if err != nil {
		if _, ok := err.(*requestError); !ok {
			ctx.Logger().Errorf("Error while getting token in callback: %v.", err)
		}

		unauthorized(
			ctx,
			"",
			invalidClaim,
			r.Host,
			fmt.Sprintf("Failed to get token in callback: %v.", err),
		)

		return
	}

	switch f.typ {
	case checkOIDCUserInfo:
		userInfo, err = f.provider.UserInfo(r.Context(), oauth2.StaticTokenSource(oauth2Token))
		if err != nil {
			// error coming from an external library and the possible error reasons are
			// not documented explicitly, so we assume that the cause is always rooted
			// in the incoming request, and only log it with a debug flag, via calling
			// unauthorized().
			unauthorized(
				ctx,
				"",
				invalidToken,
				r.Host,
				fmt.Sprintf("Failed to get userinfo: %v.", err),
			)

			return
		}
		oidcIDToken, err = f.getidtoken(oauth2Token)
		if err != nil {
			if _, ok := err.(*requestError); !ok {
				ctx.Logger().Errorf("Error while getting id token: %v", err)
			}

			unauthorized(
				ctx,
				"",
				invalidClaim,
				r.Host,
				fmt.Sprintf("Failed to get id token: %v", err),
			)

			return
		}
		sub = userInfo.Subject
		claimsMap, _, err = f.tokenClaims(ctx, oauth2Token)
		if err != nil {
			unauthorized(
				ctx,
				"",
				invalidToken,
				r.Host,
				fmt.Sprintf("Failed to get claims: %v.", err),
			)
			return
		}
	case checkOIDCAnyClaims, checkOIDCAllClaims:
		oidcIDToken, err = f.getidtoken(oauth2Token)
		if err != nil {
			if _, ok := err.(*requestError); !ok {
				ctx.Logger().Errorf("Error while getting id token: %v", err)
			}

			unauthorized(
				ctx,
				"",
				invalidClaim,
				r.Host,
				fmt.Sprintf("Failed to get id token: %v", err),
			)

			return
		}
		claimsMap, sub, err = f.tokenClaims(ctx, oauth2Token)
		if err != nil {
			if _, ok := err.(*requestError); !ok {
				ctx.Logger().Errorf("Failed to get claims with error: %v", err)
			}

			unauthorized(
				ctx,
				"",
				invalidToken,
				r.Host,
				fmt.Sprintf(
					"Failed to get claims: %s, %v",
					f.claims,
					err,
				),
			)

			return
		}
	}

	resp = tokenContainer{
		OAuth2Token: oauth2Token,
		OIDCIDToken: oidcIDToken,
		UserInfo:    userInfo,
		Subject:     sub,
		Claims:      claimsMap,
	}
	data, err = json.Marshal(resp)
	if err != nil {
		log.Errorf("Failed to serialize claims: %v.", err)
		unauthorized(
			ctx,
			"",
			invalidSub,
			r.Host,
			"Failed to serialize claims.",
		)

		return
	}

	compressedData, err := f.compressor.compress(data)
	if err != nil {
		log.Error(err)
	}
	encryptedData, err := f.encrypter.Encrypt(compressedData)
	if err != nil {
		log.Errorf("Failed to encrypt the returned oidc data: %v.", err)
		unauthorized(
			ctx,
			"",
			invalidSub,
			r.Host,
			"Failed to encrypt the returned oidc data.",
		)

		return
	}

	f.doDownstreamRedirect(ctx, encryptedData, f.getMaxAge(claimsMap), oauthState.RedirectUrl)
}

func (f *tokenOidcFilter) getMaxAge(claimsMap map[string]interface{}) time.Duration {
	maxAge := f.validity
	if exp, ok := claimsMap["exp"].(float64); ok {
		val := time.Until(time.Unix(int64(exp), 0))
		if val > time.Minute {
			maxAge = val
			log.Debugf("Setting maxAge of OIDC cookie to %s", maxAge)
		}
	}

	return maxAge
}

func (f *tokenOidcFilter) Request(ctx filters.FilterContext) {
	var (
		allowed   bool
		cookies   []*http.Cookie
		container tokenContainer
	)
	r := ctx.Request()

	// Retrieve skipperOauthOidc cookie for processing and remove it from downstream request
	rCookies := r.Cookies()
	r.Header.Del("Cookie")
	for _, cookie := range rCookies {
		if strings.HasPrefix(cookie.Name, f.cookiename) {
			cookies = append(cookies, cookie)
		} else {
			r.AddCookie(cookie)
		}
	}
	sessionCookie := mergerCookies(cookies)
	log.Debugf("Request: Cookie merged, %d chunks, len: %d", len(cookies), len(sessionCookie.String()))

	cookie, ok := f.validateCookie(sessionCookie)
	log.Debugf("Request: Cookie Validation: %v", ok)
	if !ok {
		// 5. Authorization Server sends the End-User back to the Client with an Authorization Code.
		if strings.Contains(r.URL.Path, f.redirectPath) {
			f.callbackEndpoint(ctx)
			return
		}
		// 1. Client prepares an Authentication Request containing the desired request parameters.
		// clear existing, invalid cookies
		var purgeCookies = make([]*http.Cookie, len(cookies))
		for i, c := range cookies {
			purgeCookies[i] = f.deleteOidcCookie(ctx, c.Name)
		}
		f.doOauthRedirect(ctx, purgeCookies)
		return
	}

	err := json.Unmarshal([]byte(cookie), &container)
	if err != nil {
		unauthorized(
			ctx,
			"",
			invalidToken,
			r.Host,
			fmt.Sprintf("Failed to deserialize cookie: %v.", err),
		)

		return
	}
	// filter specific checks
	switch f.typ {
	case checkOIDCUserInfo:
		if container.OAuth2Token.Valid() && container.UserInfo != nil {
			allowed = f.validateAllClaims(container.Claims)
		}
	case checkOIDCAnyClaims:
		allowed = f.validateAnyClaims(container.Claims)
	case checkOIDCAllClaims:
		allowed = f.validateAllClaims(container.Claims)
	default:
		unauthorized(ctx, "unknown", invalidFilter, r.Host, "")
		return
	}

	if !allowed {
		unauthorized(ctx, container.Subject, invalidClaim, r.Host, "")
		return
	}

	// saving token info for chained filter
	ctx.StateBag()[oidcClaimsCacheKey] = container

	// adding upstream headers
	err = setHeaders(f.upstreamHeaders, ctx, container)
	if err != nil {
		ctx.Logger().Errorf("%v", err)
		f.internalServerError(ctx)
		return
	}
}

func setHeaders(upstreamHeaders map[string]string, ctx filters.FilterContext, container interface{}) (err error) {
	oidcInfoJson, err := json.Marshal(container)
	if err != nil || !gjson.ValidBytes(oidcInfoJson) {
		return fmt.Errorf("failed to serialize OIDC token info: %w", err)
	}

	// backwards compatible
	if len(upstreamHeaders) == 0 {
		ctx.Request().Header.Set(oidcInfoHeader, string(oidcInfoJson))
		return
	}

	parsed := gjson.ParseBytes(oidcInfoJson)

	for key, query := range upstreamHeaders {
		match := parsed.Get(query)
		log.Debugf("header: %s results: %s", query, match.String())
		if !match.Exists() {
			log.Errorf("Lookup failed for upstream header '%s'", query)
			continue
		}
		ctx.Request().Header.Set(key, match.String())
	}
	return
}

func (f *tokenOidcFilter) tokenClaims(ctx filters.FilterContext, oauth2Token *oauth2.Token) (map[string]interface{}, string, error) {
	r := ctx.Request()
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, "", requestErrorf("invalid token, no id_token field in oauth2 token")
	}

	var idToken *oidc.IDToken
	idToken, err := f.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		return nil, "", requestErrorf("failed to verify id token: %v", err)
	}

	tokenMap := make(map[string]interface{})
	if err = idToken.Claims(&tokenMap); err != nil {
		return nil, "", requestErrorf("failed to deserialize id token: %v", err)
	}

	sub, ok := tokenMap["sub"].(string)
	if !ok {
		return nil, "", requestErrorf("claims do not contain sub")
	}

	if err = f.handleDistributedClaims(idToken, oauth2Token, tokenMap); err != nil {
		return nil, "", requestErrorf("failed to handle distributed claims: %v", err)
	}

	return tokenMap, sub, nil
}

func (f *tokenOidcFilter) getidtoken(oauth2Token *oauth2.Token) (string, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return "", requestErrorf("invalid token, no id_token field in oauth2 token")
	}
	return rawIDToken, nil
}

func (f *tokenOidcFilter) getCallbackState(ctx filters.FilterContext) (*OauthState, error) {
	// CSRF protection using similar to
	// https://www.owasp.org/index.php/Cross-Site_Request_Forgery_(CSRF)_Prevention_Cheat_Sheet#Encrypted_Token_Pattern,
	// because of https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest
	r := ctx.Request()
	stateQueryEncHex := r.URL.Query().Get("state")
	if stateQueryEncHex == "" {
		return nil, requestErrorf("no state parameter")
	}

	stateQueryEnc := make([]byte, len(stateQueryEncHex))
	if _, err := fmt.Sscanf(stateQueryEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		return nil, requestErrorf("failed to read hex string: %v", err)
	}

	stateQueryPlain, err := f.encrypter.Decrypt(stateQueryEnc)
	if err != nil {
		// TODO: Implement metrics counter for number of incorrect tokens
		return nil, requestErrorf("token from state query is invalid: %v", err)
	}

	ctx.Logger().Debugf("len(stateQueryPlain): %d, stateQueryEnc: %d, stateQueryEncHex: %d", len(stateQueryPlain), len(stateQueryEnc), len(stateQueryEncHex))

	state, err := extractState(stateQueryPlain)
	if err != nil {
		return nil, requestErrorf("failed to deserialize state: %v", err)
	}

	return state, nil
}

func (f *tokenOidcFilter) getTokenWithExchange(state *OauthState, ctx filters.FilterContext) (*oauth2.Token, error) {
	r := ctx.Request()
	if state.Validity < time.Now().Unix() {
		return nil, requestErrorf("state is no longer valid. %v", state.Validity)
	}

	// authcode flow
	code := r.URL.Query().Get("code")

	// https://openid.net/specs/openid-connect-core-1_0.html#CodeFlowSteps
	// 6. Client requests a response using the Authorization Code at the Token Endpoint.
	// 7. Client receives a response that contains an ID Token and Access Token in the response body.
	oauth2Token, err := f.config.Exchange(r.Context(), code, f.authCodeOptions...)
	if err != nil {
		// error coming from an external library and the possible error reasons are
		// not documented explicitly, so we assume that the cause is always rooted
		// in the incoming request.
		err = requestErrorf("oauth2 exchange: %v", err)
	}

	return oauth2Token, err
}

// handleDistributedClaims handles if user has a distributed / overage token.
// https://docs.microsoft.com/en-us/azure/active-directory/develop/id-tokens#groups-overage-claim
// In Azure, if you are indirectly member of more than 200 groups, they will
// send _claim_names and _claim_sources instead of the groups, per OIDC Core 1.0, section 5.6.2:
// https://openid.net/specs/openid-connect-core-1_0.html#AggregatedDistributedClaims
// Example:
//
//	{
//		 "_claim_names": {
//		   "groups": "src1"
//		 },
//		 "_claim_sources": {
//		   "src1": {
//		     "endpoint": "https://graph.windows.net/.../getMemberObjects"
//		   }
//	  }
//	}
func (f *tokenOidcFilter) handleDistributedClaims(idToken *oidc.IDToken, oauth2Token *oauth2.Token, claimsMap map[string]interface{}) error {
	// https://github.com/coreos/go-oidc/issues/171#issuecomment-1044286153
	var distClaims distributedClaims
	err := idToken.Claims(&distClaims)
	if err != nil {
		return err
	}
	if len(distClaims.ClaimNames) == 0 || len(distClaims.ClaimSources) == 0 {
		log.Debugf("No distributed claims found")
		return nil
	}

	for claim, ref := range distClaims.ClaimNames {
		source, ok := distClaims.ClaimSources[ref]
		if !ok {
			return fmt.Errorf("invalid distributed claims: missing claim source for %s", claim)
		}
		uri, err := url.Parse(source.Endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse distributed claim endpoint: %w", err)
		}

		var results []interface{}

		switch uri.Host {
		case azureADGraphHost, microsoftGraphHost:
			results, err = f.handleDistributedClaimsAzure(uri, oauth2Token, claimsMap)
			if err != nil {
				return fmt.Errorf("failed to get distributed Azure claim: %w", err)
			}
		default:
			return fmt.Errorf("unsupported distributed claims endpoint '%s', please create an issue at https://github.com/zalando/skipper/issues/new/choose", uri.Host)
		}

		claimsMap[claim] = results
	}
	return nil
}

// Azure customizations https://docs.microsoft.com/en-us/graph/migrate-azure-ad-graph-overview
// If the endpoints provided in _claim_source is pointed to the deprecated "graph.windows.net" api
// replace with handcrafted url to graph.microsoft.com
func (f *tokenOidcFilter) handleDistributedClaimsAzure(url *url.URL, oauth2Token *oauth2.Token, claimsMap map[string]interface{}) (values []interface{}, err error) {
	url.Host = microsoftGraphHost
	// transitiveMemberOf for group names
	userID, ok := claimsMap["oid"].(string)
	if !ok {
		return nil, fmt.Errorf("oid claim not found in claims map")
	}
	url.Path = fmt.Sprintf("/v1.0/users/%s/transitiveMemberOf", userID)
	q := url.Query()
	q.Set("$select", "onPremisesSamAccountName,id")
	url.RawQuery = q.Encode()
	return f.resolveDistributedClaimAzure(url, oauth2Token)
}

func (f *tokenOidcFilter) initClient() *snet.Client {
	newCli := snet.NewClient(snet.Options{
		ResponseHeaderTimeout:   f.oidcOptions.Timeout,
		TLSHandshakeTimeout:     f.oidcOptions.Timeout,
		MaxIdleConnsPerHost:     f.oidcOptions.MaxIdleConns,
		Tracer:                  f.oidcOptions.Tracer,
		OpentracingComponentTag: "skipper",
		OpentracingSpanName:     "distributedClaims",
	})
	return newCli
}

func (f *tokenOidcFilter) resolveDistributedClaimAzure(url *url.URL, oauth2Token *oauth2.Token) (values []interface{}, err error) {
	var target azureGraphGroups
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error constructing groups endpoint request: %w", err)
	}
	oauth2Token.SetAuthHeader(req)

	cli, ok := distributedClaimsClients.Load(url.Host)
	if !ok {
		var loaded bool
		newCli := f.initClient()
		cli, loaded = distributedClaimsClients.LoadOrStore(url.Host, newCli)
		if loaded {
			newCli.Close()
		}
	}

	client, ok := cli.(*snet.Client)
	if !ok {
		return nil, errors.New("invalid distributed claims client type")
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to call API: %w", err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close() // closing for connection reuse
	if err != nil {
		return nil, fmt.Errorf("failed to read API response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned error: %s", string(body))
	}

	err = json.Unmarshal(body, &target)
	if err != nil {
		return nil, fmt.Errorf("unable to decode response: %w", err)
	}
	for _, v := range target.Value {
		if v.OnPremisesSamAccountName != "" {
			values = append(values, v.OnPremisesSamAccountName)
		}
	}
	// recursive pagination
	if target.OdataNextLink != "" {
		nextURL, err := url.Parse(target.OdataNextLink)
		if err != nil {
			return nil, fmt.Errorf("failed to parse next link: %w", err)
		}
		vs, err := f.resolveDistributedClaimAzure(nextURL, oauth2Token)
		if err != nil {
			return nil, err
		}
		values = append(values, vs...)
	}
	log.Debugf("Distributed claim is :%v", values)
	return
}

func newDeflatePoolCompressor(level int) *deflatePoolCompressor {
	return &deflatePoolCompressor{
		poolWriter: &sync.Pool{
			New: func() interface{} {
				w, err := flate.NewWriter(io.Discard, level)
				if err != nil {
					log.Errorf("failed to generate new deflate writer: %v", err)
				}
				return w
			},
		},
	}
}

func (dc *deflatePoolCompressor) compress(rawData []byte) ([]byte, error) {
	pw, ok := dc.poolWriter.Get().(*flate.Writer)
	if !ok || pw == nil {
		return nil, fmt.Errorf("could not get a flate.Writer from the pool")
	}
	defer dc.poolWriter.Put(pw)

	var buf bytes.Buffer
	pw.Reset(&buf)

	if _, err := pw.Write(rawData); err != nil {
		return nil, err
	}
	if err := pw.Close(); err != nil {
		return nil, err
	}
	log.Debugf("cookie compressed: %d to %d", len(rawData), buf.Len())
	return buf.Bytes(), nil
}

func (dc *deflatePoolCompressor) decompress(compData []byte) ([]byte, error) {
	zr := flate.NewReader(bytes.NewReader(compData))
	if err := zr.Close(); err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}
