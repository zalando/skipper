package auth

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"fmt"
	"github.com/prometheus/common/log"
	"golang.org/x/crypto/scrypt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

//SecretSource operates on the secret for OpenID
type secretSource interface {
	GetSecret() ([][]byte, error)
}

type FileSecretSource struct {
	fileName string
}

func (fss *FileSecretSource) GetSecret() ([][]byte, error) {
	contents, err := ioutil.ReadFile(fss.fileName)
	if err != nil {
		return nil, err
	}
	secrets := strings.Split(string(contents), ",")
	byteSecrets := make([][]byte, len(secrets))
	for i, s := range secrets {
		byteSecrets[i] = []byte(s)
	}
	return byteSecrets, nil
}

func NewFileSecretSource(file string) secretSource {
	return &FileSecretSource{fileName: file}
}

type encrypter struct {
	cipherSuites []cipher.AEAD
	mux          sync.RWMutex
	sSource      secretSource
	closer       chan int
}

func NewEncrypter(secretsFile string) *encrypter {
	secretSource := NewFileSecretSource(secretsFile)
	return &encrypter{
		sSource: secretSource,
		closer:  make(chan int),
	}
}

func (c *encrypter) createNonce() ([]byte, error) {
	if len(c.cipherSuites) > 0 {
		nonce := make([]byte, c.cipherSuites[0].NonceSize())
		if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
			return nil, err
		}
		return nonce, nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// encryptDataBlock encrypts given plaintext
func (c *encrypter) encryptDataBlock(plaintext []byte) ([]byte, error) {
	if len(c.cipherSuites) > 0 {
		nonce, err := c.createNonce()
		if err != nil {
			return nil, err
		}
		c.mux.RLock()
		defer c.mux.RUnlock()
		return c.cipherSuites[0].Seal(nonce, nonce, plaintext, nil), nil
	}
	return nil, fmt.Errorf("no ciphers which can be used")
}

// decryptDataBlock decrypts given cipher text
func (c *encrypter) decryptDataBlock(cipherText []byte) ([]byte, error) {
	c.mux.RLock()
	defer c.mux.RUnlock()
	for _, c := range c.cipherSuites {
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

func (c *encrypter) refreshCiphers() error {
	secrets, err := c.sSource.GetSecret()
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
	c.mux.Lock()
	defer c.mux.Unlock()
	c.cipherSuites = suites
	return nil
}

func (c *encrypter) runCipherRefresher(refreshInterval time.Duration) error {
	err := c.refreshCiphers()
	if err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(refreshInterval)
		for {
			select {
			case <-c.closer:
				return
			case <-ticker.C:
				log.Debug("started refresh of ciphers")
				err := c.refreshCiphers()
				if err != nil {
					log.Error("failed to refresh the ciphers")
				}
				log.Debug("finished refresh of ciphers")
			}
		}
	}()
	return nil
}

func (c *encrypter) close() {
	c.closer <- 1
	close(c.closer)
}
