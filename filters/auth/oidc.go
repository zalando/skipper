package auth

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/secrets"
	"golang.org/x/oauth2"
)

const (
	OidcUserInfoName  = "oauthOidcUserInfo"
	OidcAnyClaimsName = "oauthOidcAnyClaims"
	OidcAllClaimsName = "oauthOidcAllClaims"

	oauthOidcCookieName = "skipperOauthOidc"
	stateValidity       = 1 * time.Minute
	oidcInfoHeader      = "Skipper-Oidc-Info"
	cookieMaxSize       = 4093 // common cookie size limit http://browsercookielimits.squawky.net/
)

type (
	tokenOidcSpec struct {
		typ             roleCheckType
		SecretsFile     string
		secretsRegistry secrets.EncrypterCreator
	}

	tokenOidcFilter struct {
		typ             roleCheckType
		config          *oauth2.Config
		provider        *oidc.Provider
		verifier        *oidc.IDTokenVerifier
		claims          []string
		validity        time.Duration
		cookiename      string
		redirectPath    string
		encrypter       secrets.Encryption
		authCodeOptions []oauth2.AuthCodeOption
		compressor      cookieCompression
	}

	tokenContainer struct {
		OAuth2Token *oauth2.Token          `json:"oauth2token"`
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

// NewOAuthOidcUserInfos creates filter spec which tests user info.
func NewOAuthOidcUserInfos(secretsFile string, secretsRegistry *secrets.Registry) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: secretsFile, secretsRegistry: secretsRegistry}
}

// NewOAuthOidcAnyClaims creates a filter spec which verifies that the token
// has one of the claims specified
func NewOAuthOidcAnyClaims(secretsFile string, secretsRegistry *secrets.Registry) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: secretsFile, secretsRegistry: secretsRegistry}
}

// NewOAuthOidcAllClaims creates a filter spec which verifies that the token
// has all the claims specified
func NewOAuthOidcAllClaims(secretsFile string, secretsRegistry *secrets.Registry) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: secretsFile, secretsRegistry: secretsRegistry}
}

// CreateFilter creates an OpenID Connect authorization filter.
//
// first arg: a provider, for example "https://accounts.google.com",
//            which has the path /.well-known/openid-configuration
//
// Example:
//
//     oauthOidcAllClaims("https://accounts.identity-provider.com", "some-client-id", "some-client-secret",
//     "http://callback.com/auth/provider/callback", "scope1 scope2", "claim1 claim2") -> "https://internal.example.org";
func (s *tokenOidcSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) <= 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	issuerURL, err := url.Parse(sargs[0])
	if err != nil {
		log.Errorf("Failed to parse url %s: %v.", sargs[0], err)
		return nil, filters.ErrInvalidFilterParameters
	}

	provider, err := oidc.NewProvider(context.Background(), issuerURL.String())
	if err != nil {
		log.Errorf("Failed to create new provider %s: %v.", issuerURL, err)
		return nil, filters.ErrInvalidFilterParameters
	}

	h := sha256.New()
	for _, s := range sargs {
		h.Write([]byte(s))
	}
	byteSlice := h.Sum(nil)
	sargsHash := fmt.Sprintf("%x", byteSlice)[:8]
	generatedCookieName := oauthOidcCookieName + sargsHash + "-"
	log.Debugf("Generated Cookie Name: %s", generatedCookieName)

	redirectURL, err := url.Parse(sargs[3])
	if err != nil || sargs[3] == "" {
		return nil, fmt.Errorf("invalid redirect url '%s': %v", sargs[3], err)
	}

	encrypter, err := s.secretsRegistry.GetEncrypter(1*time.Minute, s.SecretsFile)
	if err != nil {
		return nil, err
	}

	f := &tokenOidcFilter{
		typ:          s.typ,
		redirectPath: redirectURL.Path,
		config: &oauth2.Config{
			ClientID:     sargs[1],
			ClientSecret: sargs[2],
			RedirectURL:  sargs[3], // self endpoint
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID}, // mandatory scope by spec
		},
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: sargs[1],
		}),
		validity:   1 * time.Hour,
		cookiename: generatedCookieName,
		encrypter:  encrypter,
		compressor: newDeflatePoolCompressor(flate.BestCompression),
	}

	// user defined scopes
	scopes := strings.Split(sargs[4], " ")
	if len(sargs[4]) == 0 {
		scopes = []string{}
	}
	// scopes are only used to request claims to be in the IDtoken requested from auth server
	// https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims
	f.config.Scopes = append(f.config.Scopes, scopes...)
	// user defined claims to check for authnz
	if len(sargs[5]) > 0 {
		f.claims = strings.Split(sargs[5], " ")
	}

	f.authCodeOptions = make([]oauth2.AuthCodeOption, 0)
	if len(sargs) > 6 {
		extraParameters := strings.Split(sargs[6], " ")

		for _, p := range extraParameters {
			splitP := strings.Split(p, "=")
			log.Debug(splitP)
			if len(splitP) != 2 {
				return nil, filters.ErrInvalidFilterParameters
			}
			f.authCodeOptions = append(f.authCodeOptions, oauth2.SetAuthURLParam(splitP[0], splitP[1]))
		}
	}
	log.Debugf("Auth Code Options: %v", f.authCodeOptions)
	return f, nil
}

func (s *tokenOidcSpec) Name() string {
	switch s.typ {
	case checkOIDCUserInfo:
		return OidcUserInfoName
	case checkOIDCAnyClaims:
		return OidcAnyClaimsName
	case checkOIDCAllClaims:
		return OidcAllClaimsName
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

	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}

	return intersect(f.claims, keys)
}

func (f *tokenOidcFilter) validateAllClaims(h map[string]interface{}) bool {
	l := len(f.claims)
	if l == 0 {
		return true
	}
	if len(h) < l {
		return false
	}

	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return all(f.claims, keys)
}

const (
	secretSize    = 20
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var (
	src = rand.NewSource(time.Now().UnixNano())
)

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
func randString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

type OauthState struct {
	Rand        string `json:"rand"`
	Validity    int64  `json:"validity"`
	Nonce       string `json:"none"`
	RedirectUrl string `json:"redirectUrl"`
}

func createState(nonce []byte, redirectUrl string) ([]byte, error) {
	state := &OauthState{
		Rand:        randString(secretSize),
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
func (f *tokenOidcFilter) doOauthRedirect(ctx filters.FilterContext) {
	nonce, err := f.encrypter.CreateNonce()
	if err != nil {
		log.Errorf("Failed to create nonce: %v.", err)
		f.internalServerError(ctx)
		return
	}

	redirectUrl := ctx.Request().URL.String()
	statePlain, err := createState(nonce, redirectUrl)
	if err != nil {
		log.Errorf("Failed to create oauth2 state: %v.", err)
		f.internalServerError(ctx)
		return
	}
	stateEnc, err := f.encrypter.Encrypt(statePlain)
	if err != nil {
		log.Errorf("Failed to encrypt data block: %v.", err)
		f.internalServerError(ctx)
		return
	}

	oauth2URL := f.config.AuthCodeURL(fmt.Sprintf("%x", stateEnc), f.authCodeOptions...)
	rsp := &http.Response{
		Header: http.Header{
			"Location": []string{oauth2URL},
		},
		StatusCode: http.StatusTemporaryRedirect,
		Status:     "Moved Temporarily",
	}
	log.Debugf("serve redirect: plaintextState:%s to Location: %s", statePlain, rsp.Header.Get("Location"))
	ctx.Serve(rsp)
}

func (f *tokenOidcFilter) Response(filters.FilterContext) {}

func extractDomainFromHost(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	ip := net.ParseIP(h)
	if ip != nil {
		return ip.String()
	}
	if strings.Count(h, ".") < 2 {
		return h
	}
	return strings.Join(strings.Split(h, ".")[1:], ".")
}

func getHost(request *http.Request) string {
	if h := request.Header.Get("host"); h != "" {
		return h
	} else {
		return request.Host
	}
}

func chunkCookie(cookie http.Cookie) (cookies []http.Cookie) {
	for index := 'a'; index <= 'z'; index++ {
		cookieSize := len(cookie.String())
		if cookieSize < cookieMaxSize {
			cookie.Name += string(index)
			return append(cookies, cookie)
		}

		newCookie := cookie
		newCookie.Name += string(index)
		// non-deterministic approach support signature changes
		cut := len(cookie.Value) - (cookieSize - cookieMaxSize) - 1
		newCookie.Value, cookie.Value = cookie.Value[:cut], cookie.Value[cut:]
		cookies = append(cookies, newCookie)
	}
	log.Error("unsupported amount of chunked cookies")
	return
}

func mergerCookies(cookies []http.Cookie) (cookie http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	cookie = cookies[0]
	cookie.Name = cookie.Name[:len(cookie.Name)-1]
	cookie.Value = ""
	// potentially shuffeled
	sort.Slice(cookies, func(i, j int) bool {
		return cookies[i].Name < cookies[j].Name
	})
	for _, ck := range cookies {
		cookie.Value += ck.Value
	}
	return
}

func (f *tokenOidcFilter) doDownstreamRedirect(ctx filters.FilterContext, oidcState []byte, redirectUrl string) {
	log.Debugf("Doing Downstream Redirect to :%s", redirectUrl)
	r := &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{
			"Location": {redirectUrl},
		},
	}

	oidcCookies := chunkCookie(http.Cookie{
		Name:     f.cookiename,
		Value:    base64.StdEncoding.EncodeToString(oidcState),
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		MaxAge:   int(f.validity.Seconds()),
		Domain:   extractDomainFromHost(getHost(ctx.Request())),
	})
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
	)

	r := ctx.Request()
	oauthState, err := f.getCallbackState(ctx)
	if err != nil {
		if _, ok := err.(*requestError); !ok {
			log.Errorf("Error while retrieving callback state: %v.", err)
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
			log.Errorf("Error while getting token in callback: %v.", err)
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
		claimsMap, sub, err = f.tokenClaims(ctx, oauth2Token)
		if err != nil {
			if _, ok := err.(*requestError); !ok {
				log.Errorf("Failed to get claims with error: %v", err)
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

	f.doDownstreamRedirect(ctx, encryptedData, oauthState.RedirectUrl)
}

func (f *tokenOidcFilter) Request(ctx filters.FilterContext) {
	var (
		allowed   bool
		cookies   []http.Cookie
		container tokenContainer
	)
	r := ctx.Request()

	for _, cookie := range r.Cookies() {
		if strings.HasPrefix(cookie.Name, f.cookiename) {
			cookies = append(cookies, *cookie)
		}
	}
	sessionCookie := mergerCookies(cookies)
	log.Debugf("Request: Cookie merged, %d chunks, len: %d", len(cookies), len(sessionCookie.String()))

	cookie, ok := f.validateCookie(&sessionCookie)
	log.Debugf("Request: Cookie Validation: %v", ok)
	if !ok {
		// 5. Authorization Server sends the End-User back to the Client with an Authorization Code.
		if strings.Contains(r.URL.Path, f.redirectPath) {
			f.callbackEndpoint(ctx)
			return
		}
		// 1. Client prepares an Authentication Request containing the desired request parameters.
		f.doOauthRedirect(ctx)
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

	oidcInfoJson, err := json.Marshal(container)
	if err != nil {
		log.Errorf("Failed to serialize OIDC info: %v.", err)
		f.internalServerError(ctx)
		return
	}
	// saving token info for chained filter
	ctx.StateBag()[oidcClaimsCacheKey] = container
	ctx.Request().Header.Add(oidcInfoHeader, string(oidcInfoJson))
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

	return tokenMap, sub, nil
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

	log.Debugf("len(stateQueryPlain): %d, stateQueryEnc: %d, stateQueryEncHex: %d", len(stateQueryPlain), len(stateQueryEnc), len(stateQueryEncHex))

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

func (f *tokenOidcFilter) Close() error {
	f.encrypter.Close()
	return nil
}

func newDeflatePoolCompressor(level int) *deflatePoolCompressor {
	return &deflatePoolCompressor{
		poolWriter: &sync.Pool{
			New: func() interface{} {
				w, err := flate.NewWriter(ioutil.Discard, level)
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
	return ioutil.ReadAll(zr)
}
