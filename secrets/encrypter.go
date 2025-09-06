package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/scrypt"
)

type SecretSource interface {
	GetSecret() ([][]byte, error)
}

type fileSecretSource struct {
	fileName string
}

func (fss *fileSecretSource) GetSecret() ([][]byte, error) {
	contents, err := os.ReadFile(fss.fileName)
	if err != nil {
		return nil, err
	}
	secrets := strings.Split(string(contents), ",")
	byteSecrets := make([][]byte, len(secrets))
	for i, s := range secrets {
		byteSecrets[i] = []byte(s)
		if len(byteSecrets[i]) == 0 {
			return nil, fmt.Errorf("file %s secret %d is empty", fss.fileName, i)
		}
	}
	if len(byteSecrets) == 0 {
		return nil, fmt.Errorf("secrets file %s is empty", fss.fileName)
	}

	return byteSecrets, nil
}

func newFileSecretSource(file string) SecretSource {
	return &fileSecretSource{fileName: file}
}

type Encrypter struct {
	mu           sync.RWMutex
	cipherSuites []cipher.AEAD
	secretSource SecretSource
	closer       chan struct{}
	closedHook   chan struct{}
}

func newEncrypter(secretsFile string) (*Encrypter, error) {
	secretSource := newFileSecretSource(secretsFile)
	_, err := secretSource.GetSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets from secret source: %w", err)
	}
	return &Encrypter{
		secretSource: secretSource,
		closer:       make(chan struct{}),
	}, nil
}

// WithSource can be used to create an Encrypter, for example in
// secrettest for testing purposes.
func WithSource(s SecretSource) (*Encrypter, error) {
	return &Encrypter{
		secretSource: s,
		closer:       make(chan struct{}),
	}, nil
}

func (e *Encrypter) CreateNonce() ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.cipherSuites) > 0 {
		nonce := make([]byte, e.cipherSuites[0].NonceSize())
		if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
			return nil, err
		}
		return nonce, nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// Encrypt encrypts given plaintext
func (e *Encrypter) Encrypt(plaintext []byte) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.cipherSuites) > 0 {
		nonce, err := e.CreateNonce()
		if err != nil {
			return nil, err
		}
		return e.cipherSuites[0].Seal(nonce, nonce, plaintext, nil), nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// Decrypt decrypts given cipher text
func (e *Encrypter) Decrypt(cipherText []byte) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, c := range e.cipherSuites {
		nonceSize := c.NonceSize()
		if len(cipherText) < nonceSize {
			return nil, fmt.Errorf("failed to decrypt, ciphertext too short %d", len(cipherText))
		}
		nonce, input := cipherText[:nonceSize], cipherText[nonceSize:]
		data, err := c.Open(nil, nonce, input, nil)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("none of the ciphers can decrypt the data")
}

// RefreshCiphers rotates the list of cipher.AEAD initialized with
// SecretSource from the Encrypter.
func (e *Encrypter) RefreshCiphers() error {
	secrets, err := e.secretSource.GetSecret()
	if err != nil {
		return err
	}
	suites := make([]cipher.AEAD, len(secrets))
	for i, s := range secrets {

		key, err := scrypt.Key(s, []byte{}, 1<<15, 8, 1, 32)
		if err != nil {
			return fmt.Errorf("failed to create key: %w", err)
		}
		//key has to be 16 or 32 byte
		block, err := aes.NewCipher(key)
		if err != nil {
			return fmt.Errorf("failed to create new cipher: %w", err)
		}
		aesgcm, err := cipher.NewGCM(block)
		if err != nil {
			return fmt.Errorf("failed to create new GCM: %w", err)
		}
		suites[i] = aesgcm
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cipherSuites = suites
	return nil
}

func (e *Encrypter) runCipherRefresher(refreshInterval time.Duration) error {
	err := e.RefreshCiphers()
	if err != nil {
		return fmt.Errorf("failed to refresh ciphers: %w", err)
	}
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-e.closer:
				if e.closedHook != nil {
					close(e.closedHook)
				}

				return
			case <-ticker.C:
				log.Debug("started refresh of ciphers")
				err := e.RefreshCiphers()
				if err != nil {
					log.Errorf("failed to refresh the ciphers: %v", err)
				}
				log.Debug("finished refresh of ciphers")
			}
		}
	}()
	return nil
}

func (e *Encrypter) Close() {
	close(e.closer)
}
