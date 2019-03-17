package auth

import (
	"fmt"
	"io"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/stretchr/testify/assert"
)

const (
	testRedirectUrl = "http://redirect-somewhere.com/some-path?arg=param"
)

func makeTestingFilter(claims []string) (*tokenOidcFilter, error) {
	f := &tokenOidcFilter{
		typ:    checkOIDCAnyClaims,
		claims: claims,
		config: &oauth2.Config{
			ClientID: "test",
			Endpoint: google.Endpoint,
		},
		encrypter: &encrypter{
			sSource: &testingSecretSource{secretKey: "key"},
			closer:  make(chan struct{}),
		},
	}
	err := f.encrypter.refreshCiphers()
	return f, err
}

func TestEncryptDecryptState(t *testing.T) {
	f, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "could not refresh ciphers")

	nonce, err := f.encrypter.createNonce()
	if err != nil {
		t.Errorf("Failed to create nonce: %v", err)
	}

	// enc
	state, err := createState(nonce, testRedirectUrl)
	assert.NoError(t, err, "failed to create state")
	stateEnc, err := f.encrypter.encryptDataBlock(state)
	if err != nil {
		t.Errorf("Failed to encrypt data block: %v", err)
	}
	stateEncHex := fmt.Sprintf("%x", stateEnc)

	// dec
	stateQueryEnc := make([]byte, len(stateEncHex))
	if _, err := fmt.Sscanf(stateEncHex, "%x", &stateQueryEnc); err != nil && err != io.EOF {
		t.Errorf("Failed to read hex string: %v", err)
	}
	stateQueryPlain, err := f.encrypter.decryptDataBlock(stateQueryEnc)
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
	decOauthState, err := extractState(stateQueryPlain)
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

func TestExtractDomainFromHost(t *testing.T) {

	for _, ht := range []struct {
		given    string
		expected string
	}{
		{"localhost", "localhost"},
		{"localhost.localdomain", "localhost.localdomain"},
		{"www.example.local", "example.local"},
		{"one.two.three.www.example.local", "two.three.www.example.local"},
		{"localhost:9990", "localhost"},
		{"www.example.local:9990", "example.local"},
		{"127.0.0.1:9090", "127.0.0.1"},
	} {
		t.Run(fmt.Sprintf("test:%s", ht.given), func(t *testing.T) {
			got := extractDomainFromHost(ht.given)
			assert.Equal(t, ht.expected, got)
		})
	}
}
