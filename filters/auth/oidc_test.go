package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
)

const (
	testRedirectUrl = "http://redirect-somewhere.com/some-path?arg=param"
)

type testingSecretSource struct {
	getCount  int
	secretKey string
}

func makeTestingFilter(claims []string) (*tokenOidcFilter, error) {
	f := &tokenOidcFilter{
		typ:          checkOidcAnyClaims,
		secretSource: &testingSecretSource{secretKey: "abc"},
		claims:       claims,
		config: &oauth2.Config{
			ClientID: "test",
			Endpoint: google.Endpoint,
		},
	}
	err := f.refreshCiphers()
	return f, err
}

func (s *testingSecretSource) GetSecret() ([][]byte, error) {
	s.getCount++
	return [][]byte{[]byte(s.secretKey)}, nil
}

func TestEncryptDecrypt(t *testing.T) {
	f, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "could not refresh ciphers")

	plaintext := "helloworld"
	plain := []byte(plaintext)
	b, err := f.encryptDataBlock(plain)
	if err != nil {
		t.Errorf("failed to encrypt data block: %v", err)
	}
	decenc, err := f.decryptDataBlock(b)
	if err != nil {
		t.Errorf("failed to decrypt data block: %v", err)
	}
	if string(decenc) != plaintext {
		t.Errorf("decrypted plaintext is not the same as plaintext: %s", string(decenc))
	}
}

func TestEncryptDecryptState(t *testing.T) {
	f, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "could not refresh ciphers")

	nonce, err := f.createNonce()
	if err != nil {
		t.Errorf("Failed to create nonce: %v", err)
	}

	// enc
	state, err := createState(nonce, testRedirectUrl)
	assert.NoError(t, err, "failed to create state")
	stateEnc, err := f.encryptDataBlock(state)
	if err != nil {
		t.Errorf("Failed to encrypt data block: %v", err)
	}
	stateEncHex := fmt.Sprintf("%x", stateEnc)

	// dec
	stateQueryEnc := make([]byte, len(stateEncHex))
	if _, err := fmt.Sscanf(stateEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		t.Errorf("Failed to read hex string: %v", err)
	}
	stateQueryPlain, err := f.decryptDataBlock(stateQueryEnc)
	if err != nil {
		t.Errorf("token from state query is invalid: %v", err)
	}

	// test same
	if len(stateQueryPlain) != len(state) {
		t.Errorf("encoded and decoded states do no match")
	}
	for i, b := range stateQueryPlain {
		if b != state[i] {
			t.Errorf("encoded and decoded states do no match")
			break
		}
	}
	decOauthState, err := makeState(stateQueryPlain)
	if err != nil {
		t.Errorf("failed to recreate state from decrypted byte array.")
	}
	ts := time.Unix(decOauthState.Validity, 0)
	if time.Now().After(ts) {
		t.Errorf("now is after time from state but should be before: %s", ts)
	}

	if decOauthState.RedirectUrl != testRedirectUrl {
		t.Errorf("Decrypted Redirect Url %s does not match input %s", decOauthState.RedirectUrl, testRedirectUrl)
	}
}

func TestOidcValidateAllClaims(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{"uid", "email", "hd"})
	assert.NoError(t, err, "error creating test filter")
	assert.True(t, oidcFilter.validateAllClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org", "hd": "example.org"}),
		"claims should be valid but filter returned false.")
	assert.False(t, oidcFilter.validateAllClaims(
		map[string]interface{}{}), "claims are invalid but filter returned true.")
	assert.False(t, oidcFilter.validateAllClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org"}),
		"claims are invalid but filter returned true.")
	assert.True(t, oidcFilter.validateAllClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org", "hd": "something.com", "empty": ""}),
		"claims are valid but filter returned false.")
}

func TestOidcValidateAnyClaims(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{"uid", "email", "hd"})
	assert.NoError(t, err, "error creating test filter")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org", "hd": "example.org"}),
		"claims should be valid but filter returned false.")
	assert.False(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{}), "claims are invalid but filter returned true.")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org"}),
		"claims are invalid but filter returned true.")
	assert.True(t, oidcFilter.validateAnyClaims(
		map[string]interface{}{"uid": "test", "email": "test@example.org", "hd": "something.com", "empty": ""}),
		"claims are valid but filter returned false.")
}

func TestCipherRefreshing(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{})
	secretSource := &testingSecretSource{secretKey: "abc"}
	assert.NoError(t, err, "error creating filter")
	oidcFilter.closer = make(chan int)
	oidcFilter.secretSource = secretSource
	oidcFilter.runCipherRefresher(5 * time.Second)
	time.Sleep(15 * time.Second)
	oidcFilter.Close()
	assert.True(t, secretSource.getCount >= 3, "secret fetched less than 3 time in 15 seconds")
}

type TestContext struct {
	requestUrl *url.URL
}

func (t *TestContext) ResponseWriter() http.ResponseWriter {
	panic("not implemented")
}

func (t *TestContext) Request() *http.Request {
	return &http.Request{
		URL: t.requestUrl,
	}
}

func (t *TestContext) Response() *http.Response {
	panic("not implemented")
}

func (t *TestContext) OriginalRequest() *http.Request {
	panic("not implemented")
}

func (t *TestContext) OriginalResponse() *http.Response {
	panic("not implemented")
}

func (t *TestContext) Served() bool {
	panic("not implemented")
}

func (t *TestContext) MarkServed() {
	panic("not implemented")
}

func (t *TestContext) Serve(*http.Response) {
	panic("not implemented")
}

func (t *TestContext) PathParam(string) string {
	panic("not implemented")
}

func (t *TestContext) StateBag() map[string]interface{} {
	return map[string]interface{}{}
}

func (t *TestContext) BackendUrl() string {
	panic("not implemented")
}

func (t *TestContext) OutgoingHost() string {
	panic("not implemented")
}

func (t *TestContext) SetOutgoingHost(string) {
	panic("not implemented")
}

func (t *TestContext) Metrics() filters.Metrics {
	panic("not implemented")
}

func (t *TestContext) Tracer() opentracing.Tracer {
	panic("not implemented")
}

func Test_getTokenFromCookie(t *testing.T) {
	oidcFilter, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "error creating filter")
	ctx := &TestContext{}
	encrypted, err := oidcFilter.encryptDataBlock([]byte("{\"Oauth2Token\": {}, \"TokenMap\": {}}"))
	assert.NoError(t, err, "failed to encrypt data")
	cookie := oidcFilter.createCookie(encrypted)
	token, err := oidcFilter.getTokenFromCookie(ctx, cookie.Value)
	assert.NoError(t, err, "failed to get token from cookie")
	assert.NotNil(t, token.OAuth2Token, "retrieved oauth token is nil")
	assert.NotNil(t, token.TokenMap, "retrieved tokenmap is nil")
}
