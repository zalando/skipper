package auth

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
	"time"
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
	}
	err := f.refreshCiphers()
	return f, err
}

func (s *testingSecretSource) GetSecret() ([][]byte, error) {
	s.getCount += 1
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
	statePlain := createState(fmt.Sprintf("%x", nonce))
	stateEnc, err := f.encryptDataBlock([]byte(statePlain))
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
	if string(stateQueryPlain) != statePlain {
		t.Errorf("missmatch plain text")
	}
	nonceHex := fmt.Sprintf("%x", nonce)
	ts := getTimestampFromState(stateQueryPlain, len(nonceHex))
	if time.Now().After(ts) {
		t.Errorf("now is after time from state but should be before: %s", ts)
	}
}

func Test_getTimestampFromState(t *testing.T) {
	f, err := makeTestingFilter([]string{})
	assert.NoError(t, err, "could not refresh ciphers")
	nonce, err := f.createNonce()
	if err != nil {
		t.Errorf("Failed to create nonce: %v", err)
	}
	nonceHex := fmt.Sprintf("%x", nonce)
	statePlain := createState(nonceHex)

	ts := getTimestampFromState([]byte(statePlain), len(nonceHex))
	if time.Now().After(ts) {
		t.Errorf("now is after time from state but should be before: %s", ts)
	}
}

func Test_createState(t *testing.T) {
	in := "foo"
	out := createState(in)
	ts := fmt.Sprintf("%d", time.Now().Add(1*time.Minute).Unix())
	if len(out) != len(in)+len(ts)+secretSize {
		t.Errorf("createState returned string size is wrong: %s", out)
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
