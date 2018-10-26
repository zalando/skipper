package auth

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

type testingSecretSource struct {
	getCount  int
	secretKey string
}

func (s *testingSecretSource) GetSecret() ([][]byte, error) {
	s.getCount++
	return [][]byte{[]byte(s.secretKey)}, nil
}

func TestEncryptDecrypt(t *testing.T) {
	enc := &encrypter{
		sSource: &testingSecretSource{secretKey: "abc"},
	}
	enc.refreshCiphers()

	plaintext := "helloworld"
	plain := []byte(plaintext)
	b, err := enc.encryptDataBlock(plain)
	if err != nil {
		t.Errorf("failed to encrypt data block: %v", err)
	}
	decenc, err := enc.decryptDataBlock(b)
	if err != nil {
		t.Errorf("failed to decrypt data block: %v", err)
	}
	if string(decenc) != plaintext {
		t.Errorf("decrypted plaintext is not the same as plaintext: %s", string(decenc))
	}
}
func TestCipherRefreshing(t *testing.T) {
	sSource := &testingSecretSource{secretKey: "abc"}
	enc := &encrypter{
		sSource: sSource,
		closer:  make(chan int),
	}
	enc.runCipherRefresher(1 * time.Second)
	time.Sleep(4 * time.Second)
	enc.close()
	assert.True(t, sSource.getCount >= 3, "secret fetched less than 3 time in 15 seconds")
}
