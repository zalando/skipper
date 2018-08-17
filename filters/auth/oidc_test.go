package auth

import (
	"fmt"
	"io"
	"testing"
	"time"
)

func TestEncryptDecrypt(t *testing.T) {
	aesgcm, err := getCiphersuite()
	if err != nil {
		t.Errorf("failed to create ciphersuite: %v", err)
	}

	f := &tokenOidcFilter{
		typ:  checkOidcAnyClaims,
		aead: aesgcm,
	}

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
	aesgcm, err := getCiphersuite()
	if err != nil {
		t.Errorf("failed to create ciphersuite: %v", err)
	}

	f := &tokenOidcFilter{
		typ:  checkOidcAnyClaims,
		aead: aesgcm,
	}

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
	aesgcm, err := getCiphersuite()
	if err != nil {
		t.Errorf("failed to create ciphersuite: %v", err)
	}

	f := &tokenOidcFilter{
		typ:  checkOidcAnyClaims,
		aead: aesgcm,
	}
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
