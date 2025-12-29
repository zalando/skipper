package secrets_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/zalando/skipper/secrets/secrettest"
)

func TestEncryptDecrypt(t *testing.T) {
	s := "mysecret"
	tr := secrettest.NewTestRegistry()
	enc, err := tr.GetEncrypter(10*time.Minute, "TestEncryptDecrypt")
	if err != nil {
		t.Fatalf("Failed to create test Encrpyter: %v", err)
	}

	b := []byte(s)
	encB, err := enc.Encrypt(b)
	if err != nil {
		t.Errorf("Failed to encrypt data: %v", err)
	}
	if reflect.DeepEqual(b, encB) {
		t.Error("Encrypted data has the same byte sequence as the data itself")
	}

	decB, err := enc.Decrypt(encB)
	if err != nil {
		t.Errorf("Failed to decrypt data: %v", err)
	}
	if !reflect.DeepEqual(b, decB) {
		t.Error("Decrypted and encrypted data has to be the same byte sequence as the data itself")
	}
}

func TestCreateNonce(t *testing.T) {
	tr := secrettest.NewTestRegistry()
	enc, err := tr.GetEncrypter(10*time.Minute, "TestCreateNonce")
	if err != nil {
		t.Fatalf("Failed to create test Encrpyter: %v", err)
	}

	b, err := enc.CreateNonce()
	if err != nil {
		t.Fatalf("Failed to create nonce: %v", err)
	}

	b1, err := enc.CreateNonce()
	if err != nil {
		t.Fatalf("Failed to create nonce: %v", err)
	}

	if reflect.DeepEqual(b, b1) {
		t.Error("Nonce should not be the same called twice")
	}
}
