package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"golang.org/x/oauth2"
)

const (
	OidcUserInfoName  = "oauthOidcUserInfo"
	OidcAnyClaimsName = "oauthOidcAnyClaims"
	OidcAllClaimsName = "oauthOidcAllClaims"

	oauthOidcCookieName = "skipperOauthOidc"
	stateValidity       = 1 * time.Minute
)

type (
	tokenOidcSpec struct {
		typ         roleCheckType
		SecretsFile string
	}

	tokenOidcFilter struct {
		typ             roleCheckType
		config          *oauth2.Config
		provider        *oidc.Provider
		verifier        *oidc.IDTokenVerifier
		claims          []string
		validity        time.Duration
		secretsFile     string
		cookiename      string
		refreshInterval time.Duration
		closer          chan int
		redirectPath    string
		encrypter       *encrypter
		authCodeOptions []oauth2.AuthCodeOption
	}

	userInfoContainer struct {
		OAuth2Token *oauth2.Token
		UserInfo    *oidc.UserInfo
		Subject     string
	}

	claimsContainer struct {
		OAuth2Token *oauth2.Token
		Claims      map[string]interface{}
		Subject     string
	}
)

// NewOAuthOidcUserInfos creates filter spec which tests user info.
func NewOAuthOidcUserInfos(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCUserInfo, SecretsFile: secretsFile}
}

// NewOAuthOidcAnyClaims creates a filter spec which verifies that the token
// has one of the claims specified
func NewOAuthOidcAnyClaims(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAnyClaims, SecretsFile: secretsFile}
}

// NewOAuthOidcAllClaims creates a filter spec which verifies that the token
// has all the claims specified
func NewOAuthOidcAllClaims(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOIDCAllClaims, SecretsFile: secretsFile}
}

// CreateFilter creates an OpenID Connect authorization filter.
//
// first arg: a provider, for example "https://accounts.google.com",
//            which has the path /.well-known/openid-configuration
//
// Example:
//
//     tokenOidcSpec(""https://identity-provider.example.com", "client-id", "clent-secret", "https://callback-url")  -> "https://internal.example.org";
func (s *tokenOidcSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) <= 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	providerURL, err := url.Parse(sargs[0])

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, providerURL.String())
	if err != nil {
		log.Errorf("Failed to create new provider %s: %v", providerURL, err)
		return nil, filters.ErrInvalidFilterParameters
	}

	if err != nil {
		log.Errorf("Failed to create ciphersuite: %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}
	h := sha256.New()
	for _, s := range sargs {
		h.Write([]byte(s))
	}
	byteSlice := h.Sum(nil)
	sargsHash := fmt.Sprintf("%x", byteSlice)[:8]
	generatedCookieName := oauthOidcCookieName + sargsHash
	log.Debugf("Generated Cookie Name: %s", generatedCookieName)

	redirectURL, err := url.Parse(sargs[3])
	if err != nil {
		return nil, fmt.Errorf("the redirect url %s is not valid: %v", sargs[3], err)
	}
	encrypter, err := newEncrypter(s.SecretsFile)

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
			Scopes:       []string{oidc.ScopeOpenID},
		},
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: sargs[1],
		}),
		validity:   1 * time.Hour,
		cookiename: generatedCookieName,
		encrypter:  encrypter,
	}

	// Start the self-refreshing cipher function
	err = f.encrypter.runCipherRefresher(1 * time.Minute)
	if err != nil {
		return nil, err
	}

	switch f.typ {
	case checkOIDCUserInfo:
		f.config.Scopes = append(f.config.Scopes, sargs[4:]...)
	case checkOIDCAnyClaims:
		fallthrough
	case checkOIDCAllClaims:
		additionScopes := sargs[4]
		f.config.Scopes = append(f.config.Scopes, additionScopes)
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

	var a []string
	for k := range h {
		a = append(a, k)
	}

	log.Debugf("intersect(%v, %v)", f.claims, a)
	return intersect(f.claims, a)
}

func (f *tokenOidcFilter) validateAllClaims(h map[string]interface{}) bool {
	if len(f.claims) == 0 {
		return true
	}

	var a []string
	for k := range h {
		a = append(a, k)
	}

	log.Infof("all(%v, %v)", f.claims, a)
	return all(f.claims, a)
}

const (
	secretSize    = 20
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var (
	src      = rand.NewSource(time.Now().UnixNano())
	stateMap = make(map[string]bool)
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

func (f *tokenOidcFilter) doOauthRedirect(ctx filters.FilterContext) {
	nonce, err := f.encrypter.createNonce()
	if err != nil {
		log.Errorf("Failed to create nonce: %v", err)
		f.internalServerError(ctx)
		return
	}

	redirectUrl := ctx.Request().URL.String()
	statePlain, err := createState(nonce, redirectUrl)
	if err != nil {
		log.Errorf("failed to create oauth2 state: %v", err)
		f.internalServerError(ctx)
		return
	}
	stateEnc, err := f.encrypter.encryptDataBlock(statePlain)
	if err != nil {
		log.Errorf("Failed to encrypt data block: %v", err)
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
	log.Infof("serve redirect: plaintextState:%s to Location: %s", statePlain, rsp.Header.Get("Location"))
	ctx.Serve(rsp)
}

func (f *tokenOidcFilter) Response(filters.FilterContext) {}

func (f *tokenOidcFilter) doDownstreamRedirect(ctx filters.FilterContext, oidcState []byte, redirectUrl string) {
	log.Debugf("Doing Downstream Redirect to :%s", redirectUrl)
	r := &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Header: map[string][]string{
			"Set-Cookie": {fmt.Sprintf("%s=%x; Path=/; HttpOnly; MaxAge=%d", f.cookiename, oidcState, int(f.validity.Seconds()))},
			"Location":   {redirectUrl},
		},
	}
	ctx.Serve(r)
}

func (f *tokenOidcFilter) validateCookie(cookie *http.Cookie) ([]byte, bool) {
	if cookie == nil {
		log.Debugf("Cookie is nil")
		return nil, false
	}
	log.Debugf("validate cookie name: %s", f.cookiename)
	var cookieStr string
	fmt.Sscanf(cookie.Value, "%x", &cookieStr)

	decryptedCookie, err := f.encrypter.decryptDataBlock([]byte(cookieStr))
	if err != nil {
		log.Debugf("Decrypting the cookie failed: %v", err)
		return nil, false
	}
	return []byte(decryptedCookie), true
}

func (f *tokenOidcFilter) Request(ctx filters.FilterContext) {
	var (
		oauth2Token *oauth2.Token
	)

	r := ctx.Request()
	sessionCookie, _ := r.Cookie(f.cookiename)
	cookie, ok := f.validateCookie(sessionCookie)
	var (
		data []byte
	)
	log.Debugf("Cookie Validation: %v", ok)
	if !ok {
		if strings.Contains(ctx.Request().URL.Path, f.redirectPath) {
			oauthState, err := f.getCallbackState(ctx)
			if err != nil {
				unauthorized(ctx, "failed to get state from callback: "+err.Error(), invalidToken, r.Host)
				return
			}
			oauth2Token, err = f.getTokenWithExchange(oauthState, ctx)
			if err != nil {
				unauthorized(ctx, "Failed to get token in callback: "+err.Error(), invalidToken, r.Host)
				return
			}

			switch f.typ {
			case checkOIDCUserInfo:
				userInfo, err := f.provider.UserInfo(r.Context(), oauth2.StaticTokenSource(oauth2Token))
				if err != nil {
					unauthorized(ctx, "Failed to get userinfo: "+err.Error(), invalidToken, r.Host)
					return
				}
				sub := userInfo.Subject
				resp := userInfoContainer{oauth2Token, userInfo, sub}
				data, err = json.Marshal(resp)
				if err != nil {
					unauthorized(ctx, fmt.Sprintf("Failed to marshal userinfo backend data for sub=%s: %v", sub, err), invalidToken, r.Host)
					return
				}
			case checkOIDCAnyClaims:
				fallthrough
			case checkOIDCAllClaims:
				tokenMap, sub, err := f.tokenClaims(ctx, oauth2Token)
				if err != nil {
					log.Debugf("Failed to get claims: %v", err)
					unauthorized(ctx, fmt.Sprintf("received token does not contain the claims %s", f.claims), invalidToken, r.Host)
					return
				}
				resp := claimsContainer{OAuth2Token: oauth2Token, Claims: tokenMap, Subject: sub}
				data, err = json.Marshal(resp)
				if err != nil {
					unauthorized(ctx, "failed to serialize claims", invalidSub, r.Host)
					return
				}
			}
			encryptedData, err := f.encrypter.encryptDataBlock(data)
			if err != nil {
				unauthorized(ctx, "failed to encrypt the returned oidc data", invalidSub, r.Host)
				return
			}
			f.doDownstreamRedirect(ctx, encryptedData, oauthState.RedirectUrl)
			return
		}
		f.doOauthRedirect(ctx)
		return
	}

	var (
		allowed bool
		sub     string
	)

	// filter specific checks
	switch f.typ {
	case checkOIDCUserInfo:
		var container userInfoContainer
		err := json.Unmarshal([]byte(cookie), &container)
		if err != nil {
			unauthorized(ctx, "failed to deserialize cookie: "+err.Error(), invalidToken, r.Host)
			return
		}
		if container.OAuth2Token.Valid() && container.UserInfo != nil {
			allowed = true
		}
		sub = container.Subject
	case checkOIDCAnyClaims:
		var container claimsContainer
		err := json.Unmarshal([]byte(cookie), &container)
		if err != nil {
			unauthorized(ctx, "failed to deserialize cookie: "+err.Error(), invalidToken, r.Host)
			return
		}
		allowed = f.validateAnyClaims(container.Claims)
		log.Infof("validateAnyClaims: %v", allowed)
		sub = container.Subject
	case checkOIDCAllClaims:
		var container claimsContainer
		err := json.Unmarshal([]byte(cookie), &container)
		if err != nil {
			unauthorized(ctx, "failed to deserialize cookie: "+err.Error(), invalidToken, r.Host)
			return
		}
		allowed = f.validateAllClaims(container.Claims)
		log.Infof("validateAllClaims: %v", allowed)
		sub = container.Subject
	default:
		unauthorized(ctx, "unknown", invalidFilter, r.Host)
		return
	}

	if !allowed {
		log.Infof("unauthorized")
		unauthorized(ctx, "insufficient permissions :"+sub, invalidClaim, r.Host)
		return
	}

	log.Infof("send authorized")
	authorized(ctx, sub)
}

func (f *tokenOidcFilter) tokenClaims(ctx filters.FilterContext, oauth2Token *oauth2.Token) (map[string]interface{}, string, error) {
	r := ctx.Request()
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, "", fmt.Errorf("invalid token, no id_token field in oauth2 token")
	}

	var idToken *oidc.IDToken
	idToken, err := f.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		return nil, "", err
	}

	tokenMap := make(map[string]interface{})
	if err = idToken.Claims(&tokenMap); err != nil {
		return nil, "", err
	}

	sub, ok := tokenMap["sub"].(string)
	if !ok {
		return nil, "", fmt.Errorf("claims does not contain sub")
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
		return nil, fmt.Errorf("no state parameter")
	}

	stateQueryEnc := make([]byte, len(stateQueryEncHex))
	if _, err := fmt.Sscanf(stateQueryEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		log.Errorf("Failed to read hex string: %v", err)
		return nil, err
	}

	stateQueryPlain, err := f.encrypter.decryptDataBlock(stateQueryEnc)
	if err != nil {
		log.Errorf("token from state query is invalid: %v", err)
		return nil, err
	}

	log.Debugf("len(stateQueryPlain): %d, stateQueryEnc: %d, stateQueryEncHex: %d", len(stateQueryPlain), len(stateQueryEnc), len(stateQueryEncHex))

	state, err := extractState(stateQueryPlain)
	if err != nil {
		log.Errorf("Failed to deserialize state")
		return nil, err
	}
	return state, nil
}

func (f *tokenOidcFilter) getTokenWithExchange(state *OauthState, ctx filters.FilterContext) (*oauth2.Token, error) {
	r := ctx.Request()

	if state.Validity < time.Now().Unix() {
		log.Errorf("state is no longer valid. %v", state.Validity)
		return nil, fmt.Errorf("validity of state expired")
	}

	// authcode flow
	code := r.URL.Query().Get("code")
	oauth2Token, err := f.config.Exchange(r.Context(), code, f.authCodeOptions...)
	if err != nil {
		unauthorized(ctx, "Failed to exchange token: "+err.Error(), invalidClaim, r.Host)
		return nil, err
	}
	return oauth2Token, err
}
