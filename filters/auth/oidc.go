package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/coreos/go-oidc"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/oauth2"
)

const (
	OidcUserInfoName  = "oauthOidcUserInfo"
	OidcAnyClaimsName = "oauthOidcAnyClaims"
	OidcAllClaimsName = "oauthOidcAllClaims"

	oidcStatebagKey     = "oauthOidcKey"
	oauthOidcCookieName = "skipperOauthOidc"
	refreshInterval     = 30 * time.Minute
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
		secretSource    SecretSource
		mux             sync.RWMutex
		cipherSuites    []cipher.AEAD
		refreshInterval time.Duration
		closer          chan int
	}
)

// NewOAuthOidcUserInfos creates filter spec which tests user info.
func NewOAuthOidcUserInfos(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOidcUserInfos, SecretsFile: secretsFile}
}

// NewOAuthOidcAnyClaims creates a filter spec which verifies that the token
// has one of the claims specified
func NewOAuthOidcAnyClaims(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOidcAnyClaims, SecretsFile: secretsFile}
}

// NewOAuthOidcAllClaims creates a filter spec which verifies that the token
// has all the claims specified
func NewOAuthOidcAllClaims(secretsFile string) filters.Spec {
	return &tokenOidcSpec{typ: checkOidcAllClaims, SecretsFile: secretsFile}
}

// CreateFilter creates an OpenID Connect authorization filter.
//
// first arg: a provider, for example "https://accounts.google.com",
//            which has the path /.well-known/openid-configuration
//
// Example:
//
//     tokenOidcSpec("https://accounts.google.com", "255788903420-c68l9ustnfqkvukessbn46d92tirvh6s.apps.googleusercontent.com", "hjY8LHp9bPe97hS0aqXGh_zL", "http://127.0.0.1:5556/auth/google/callback")
func (s *tokenOidcSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) < 4 {
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

	secretSource := NewFileSecretSource(s.SecretsFile)

	f := &tokenOidcFilter{
		typ: s.typ,
		config: &oauth2.Config{
			ClientID:     sargs[1],
			ClientSecret: sargs[2],
			RedirectURL:  sargs[3], // self endpoint
			Endpoint:     provider.Endpoint(),
		},
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{
			ClientID: sargs[1],
		}),
		validity:        1 * time.Hour,
		cookiename:      oauthOidcCookieName + sargsHash,
		secretSource:    secretSource,
		refreshInterval: refreshInterval,
	}
	f.config.Scopes = []string{oidc.ScopeOpenID}

	switch f.typ {
	case checkOidcUserInfos:
		if len(sargs) > 4 { // google IAM needs a scope to be sent
			f.config.Scopes = append(f.config.Scopes, sargs[4:]...)
		} else {
			// Scope check is required for auth code flow
			return nil, filters.ErrInvalidFilterParameters
		}
	case checkOidcAnyClaims:
		fallthrough
	case checkOidcAllClaims:
		f.config.Scopes = append(f.config.Scopes, sargs[4:]...)
		f.claims = sargs[4:]
	}
	f.closer = make(chan int)
	f.runCipherRefresher(refreshInterval)
	return f, nil
}

func (s *tokenOidcSpec) Name() string {
	switch s.typ {
	case checkOidcUserInfos:
		return OidcUserInfoName
	case checkOidcAnyClaims:
		return OidcAnyClaimsName
	case checkOidcAllClaims:
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

func (f *tokenOidcFilter) refreshCiphers() error {
	secrets, err := f.secretSource.GetSecret()
	if err != nil {
		return err
	}
	suites := make([]cipher.AEAD, len(secrets))
	for i, s := range secrets {

		key, err := scrypt.Key(s, []byte{}, 1<<15, 8, 1, 32)
		if err != nil {
			return fmt.Errorf("failed to create key: %v", err)
		}
		//key has to be 16 or 32 byte
		block, err := aes.NewCipher(key)
		if err != nil {
			return fmt.Errorf("failed to create new cipher: %v", err)
		}
		aesgcm, err := cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("failed to create new GCM: %v", err)
		}
		suites[i] = aesgcm
	}
	f.mux.Lock()
	defer f.mux.Unlock()
	f.cipherSuites = suites
	return nil
}

func getTimestampFromState(b []byte, nonceLength int) time.Time {
	log.Debugf("getTimestampFromState b: %s", b)
	if len(b) <= secretSize+nonceLength || secretSize >= len(b)-nonceLength {
		log.Debugf("wrong b: %d, %d, %d, b[%d : %d], %v %v", len(b), secretSize, nonceLength, secretSize, len(b)-nonceLength, len(b) <= secretSize+nonceLength, secretSize >= len(b)-nonceLength)
		return time.Time{}.Add(1 * time.Second)
	}
	ts := string(b[secretSize : len(b)-nonceLength])
	i, err := strconv.Atoi(ts)
	if err != nil {
		log.Errorf("Atoi failed: %v", err)
		return time.Time{}
	}
	return time.Unix(int64(i), 0)

}

func createState(nonce string) string {
	return randString(secretSize) + fmt.Sprintf("%d", time.Now().Add(stateValidity).Unix()) + nonce
}

func (f *tokenOidcFilter) doRedirect(ctx filters.FilterContext) {
	nonce, err := f.createNonce()
	if err != nil {
		log.Errorf("Failed to create nonce: %v", err)
		return
	}

	statePlain := createState(fmt.Sprintf("%x", nonce))
	stateEnc, err := f.encryptDataBlock([]byte(statePlain))
	if err != nil {
		log.Errorf("Failed to encrypt data block: %v", err)
	}

	rsp := &http.Response{
		Header: http.Header{
			"Location": []string{f.config.AuthCodeURL(fmt.Sprintf("%x", stateEnc))},
		},
		StatusCode: http.StatusFound,
		Status:     "Moved Temporarily",
	}
	log.Infof("serve redirect: plaintextState:%s to Location: %s", statePlain, rsp.Header.Get("Location"))
	ctx.Serve(rsp)
}

// Response saves our state bag in a cookie, such that we can get it
// back in subsequent requests to handle the requests.
func (f *tokenOidcFilter) Response(ctx filters.FilterContext) {
	//host := ctx.Request().Host
	//ctx.Request().URL.Hostname()
	if v, ok := ctx.StateBag()[oidcStatebagKey]; ok {
		cookie := &http.Cookie{
			Name:  f.cookiename,
			Value: fmt.Sprintf("%x", v),
			//Secure:   true,  // https only
			Secure:   false, // for development
			HttpOnly: false,
			Path:     "/",
			Domain:   "127.0.0.1",
			MaxAge:   int(f.validity.Seconds()),
			Expires:  time.Now().Add(f.validity),
		}
		log.Debugf("Response SetCookie: %s", cookie)
		http.SetCookie(ctx.ResponseWriter(), cookie)
	}
}

func (f *tokenOidcFilter) validateCookie(cookie *http.Cookie) (string, bool) {
	if cookie == nil {
		return "", false
	}
	log.Debugf("validate cookie name: %s", f.cookiename)

	// TODO check validity

	return cookie.Value, true
}

type tokenContainer struct {
	OAuth2Token *oauth2.Token          `json:"OAuth2Token"`
	TokenMap    map[string]interface{} `json:"TokenMap"`
}

func (f *tokenOidcFilter) Request(ctx filters.FilterContext) {
	var (
		oauth2Token *oauth2.Token
		atoken      *tokenContainer
		err         error
	)

	r := ctx.Request()
	sessionCookie, _ := r.Cookie(f.cookiename)
	cValueHex, ok := f.validateCookie(sessionCookie)

	if ok {
		log.Debugf("got valid cookie: %d", len(cValueHex))
		atoken, err := f.getTokenFromCookie(ctx, cValueHex)
		if err != nil {
			f.doRedirect(ctx)
		}
		oauth2Token = atoken.OAuth2Token

	} else {
		oauth2Token, err = f.getTokenWithExchange(ctx)
		if err != nil {
			f.doRedirect(ctx)
			return
		}

	}

	if !oauth2Token.Valid() {
		unauthorized(ctx, "invalid token", invalidToken, r.Host)
		return
	}

	var (
		allowed  bool
		data     []byte
		sub      string
		tokenMap map[string]interface{}
	)
	// filter specific checks
	switch f.typ {
	case checkOidcUserInfos:
		userInfo, err := f.provider.UserInfo(r.Context(), oauth2.StaticTokenSource(oauth2Token))
		if err != nil {
			unauthorized(ctx, "Failed to get userinfo: "+err.Error(), invalidToken, r.Host)
			return
		}
		sub = userInfo.Subject

		resp := struct {
			OAuth2Token *oauth2.Token
			UserInfo    *oidc.UserInfo
		}{oauth2Token, userInfo}
		data, err = json.Marshal(resp)
		if err != nil {
			unauthorized(ctx, fmt.Sprintf("Failed to marshal userinfo backend data for sub=%s: %v", sub, err), invalidToken, r.Host)
			return
		}

		allowed = true

	case checkOidcAnyClaims:
		tokenMap, data, err = f.oidcClaimsHandling(ctx, oauth2Token, atoken)
		if err != nil {
			return
		}
		allowed = f.validateAnyClaims(tokenMap)
		log.Infof("validateAnyClaims: %v", allowed)

	case checkOidcAllClaims:
		tokenMap, data, err = f.oidcClaimsHandling(ctx, oauth2Token, atoken)
		if err != nil {
			return
		}
		allowed = f.validateAllClaims(tokenMap)
		log.Infof("validateAllClaims: %v", allowed)

	default:
		unauthorized(ctx, "unknown", invalidFilter, r.Host)
		return
	}

	if !allowed {
		log.Infof("unauthorized")
		// TODO(sszuecs) review error handling
		unauthorized(ctx, sub, invalidClaim, r.Host)
		return
	}

	encryptedData, err := f.encryptDataBlock(data)
	if err != nil {
		log.Errorf("Failed to encrypt: %v", err)
	}

	// if we do not have a session cookie, set one in the response
	if sessionCookie == nil {
		ctx.StateBag()[oidcStatebagKey] = encryptedData
	}

	log.Infof("send authorized")
	authorized(ctx, sub)
}

func (f *tokenOidcFilter) createNonce() ([]byte, error) {
	if len(f.cipherSuites) > 0 {
		nonce := make([]byte, f.cipherSuites[0].NonceSize())
		if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
			return nil, err
		}
		return nonce, nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// encryptDataBlock encrypts given plaintext
func (f *tokenOidcFilter) encryptDataBlock(plaintext []byte) ([]byte, error) {
	if len(f.cipherSuites) > 0 {
		nonce, err := f.createNonce()
		if err != nil {
			return nil, err
		}
		f.mux.RLock()
		defer f.mux.RUnlock()
		return f.cipherSuites[0].Seal(nonce, nonce, plaintext, nil), nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// decryptDataBlock decrypts given cipher text
func (f *tokenOidcFilter) decryptDataBlock(cipherText []byte) ([]byte, error) {
	f.mux.RLock()
	defer f.mux.RUnlock()
	for _, c := range f.cipherSuites {
		nonceSize := c.NonceSize()
		if len(cipherText) < nonceSize {
			return nil, errors.New("failed to decrypt, ciphertext too short")
		}
		nonce, input := cipherText[:nonceSize], cipherText[nonceSize:]
		data, err := c.Open(nil, nonce, input, nil)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("none of the ciphers can decrypt the data")
}

// TODO think about naming or splitting
func (f *tokenOidcFilter) oidcClaimsHandling(ctx filters.FilterContext, oauth2Token *oauth2.Token, atoken *tokenContainer) (tokenMap map[string]interface{}, data []byte, err error) {

	if atoken == nil {
		r := ctx.Request()
		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			unauthorized(ctx, "No id_token field in oauth2 token", invalidToken, r.Host)
			err = fmt.Errorf("invalid token, no id_token field in oauth2 token")
			return
		}

		var idToken *oidc.IDToken
		idToken, err = f.verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			unauthorized(ctx, "Failed to verify ID Token: "+err.Error(), invalidToken, r.Host)
			return
		}

		tokenMap = make(map[string]interface{})
		if err = idToken.Claims(&tokenMap); err != nil {
			unauthorized(ctx, "Failed to get claims: "+err.Error(), invalidToken, r.Host)
			return
		}

		sub, ok := tokenMap["sub"].(string)
		if !ok {
			unauthorized(ctx, "Failed to get sub", invalidToken, r.Host)
			return
		}

		resp := struct {
			OAuth2Token *oauth2.Token
			TokenMap    map[string]interface{}
		}{oauth2Token, tokenMap}
		data, err = json.Marshal(resp)
		if err != nil {
			unauthorized(ctx, fmt.Sprintf("Failed to prepare data for backend with sub=%s: %v", sub, err), invalidToken, r.Host)
			return
		}

	} else {
		// token from cookie restored
		// TODO check validity
		tokenMap = atoken.TokenMap
	}

	return
}

func (f *tokenOidcFilter) getTokenFromCookie(ctx filters.FilterContext, cValueHex string) (*tokenContainer, error) {
	cValue := make([]byte, len(cValueHex))
	if _, err := fmt.Sscanf(cValueHex, "%x", &cValue); err != nil && err != io.EOF {
		log.Errorf("Failed to read hex string: %v", err)
		return nil, err
	}

	cValuePlain, err := f.decryptDataBlock(cValue)
	if err != nil {
		log.Errorf("token from Cookie is invalid: %v", err)
		return nil, err
	}

	atoken := &tokenContainer{}
	err = json.Unmarshal(cValuePlain, atoken)
	if err != nil {
		log.Errorf("Failed to unmarshal decrypted cookie: %v", err)
	}

	return atoken, err
}

func (f *tokenOidcFilter) getTokenWithExchange(ctx filters.FilterContext) (*oauth2.Token, error) {
	// CSRF protection using similar to
	// https://www.owasp.org/index.php/Cross-Site_Request_Forgery_(CSRF)_Prevention_Cheat_Sheet#Encrypted_Token_Pattern,
	// because of https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest
	r := ctx.Request()
	stateQueryEncHex := r.URL.Query().Get("state")
	if stateQueryEncHex == "" {
		return nil, fmt.Errorf("no state query")
	}

	stateQueryEnc := make([]byte, len(stateQueryEncHex))
	if _, err := fmt.Sscanf(stateQueryEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		log.Errorf("Failed to read hex string: %v", err)
		return nil, err
	}

	stateQueryPlain, err := f.decryptDataBlock(stateQueryEnc)
	if err != nil {
		log.Errorf("token from state query is invalid: %v", err)
		return nil, err
	}
	log.Debugf("len(stateQueryPlain): %d, stateQueryEnc: %d, stateQueryEncHex: %d", len(stateQueryPlain), len(stateQueryEnc), len(stateQueryEncHex))

	nonce, err := f.createNonce()
	if err != nil {
		log.Errorf("Failed to create nonce: %v", err)
		return nil, err
	}
	nonceHex := fmt.Sprintf("%x", nonce)
	ts := getTimestampFromState(stateQueryPlain, len(nonceHex))
	if time.Now().After(ts) {
		// state query is older than allowed -> enforce login
		return nil, fmt.Errorf("token from state query is too old: %s", ts)

	}

	// authcode flow
	code := r.URL.Query().Get("code")
	oauth2Token, err := f.config.Exchange(r.Context(), code)
	if err != nil {
		unauthorized(ctx, "Failed to exchange token: "+err.Error(), invalidClaim, r.Host)
		return nil, err
	}

	return oauth2Token, err
}

func (f *tokenOidcFilter) runCipherRefresher(refreshInterval time.Duration) error {
	err := f.refreshCiphers()
	if err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(refreshInterval)
		for {
			select {
			case <-f.closer:
				return
			case <-ticker.C:
				log.Debug("started refresh of ciphers")
				err := f.refreshCiphers()
				if err != nil {
					log.Error("failed to refresh the ciphers")
				}
				log.Debug("finished refresh of ciphers")
			}
		}
	}()
	return nil
}

func (f *tokenOidcFilter) Close() {
	f.closer <- 1
	close(f.closer)
}
