package secrets

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	ErrAlreadyExists    = errors.New("secret already exists")
	ErrWrongFileType    = errors.New("file type not supported")
	ErrFailedToReadFile = errors.New("failed to read file")
)

// SecretsReader is able to get a secret
type SecretsReader interface {
	// GetSecret finds secret by name and returns secret and if found or not
	GetSecret(string) ([]byte, bool)
}

// SecretsProvider is a SecretsReader and can add secret sources that
// contain a secret. It will automatically update secrets if the source
// changed.
type SecretsProvider interface {
	SecretsReader
	// Add adds the given source that contains a secret to the
	// automatically updated secrets store
	Add(string) error
}

type SecretPaths struct {
	mu      sync.RWMutex
	quit    chan struct{}
	secrets map[string][]byte
	files   map[string]string
}

// NewSecretPaths creates a SecretPaths, that implements a
// SecretsProvider. It runs every d interval background refresher as a
// side effect. On tear down make sure to Close() it.
func NewSecretPaths(d time.Duration) *SecretPaths {
	sp := &SecretPaths{
		quit:    make(chan struct{}),
		secrets: make(map[string][]byte),
		files:   make(map[string]string),
	}
	go sp.runRefresher(d)

	return sp
}

// GetSecret returns secret and if found or not for a given name.
func (sp *SecretPaths) GetSecret(s string) ([]byte, bool) {
	sp.mu.RLock()
	dat, ok := sp.secrets[s]
	sp.mu.RUnlock()
	return dat, ok
}

func (sp *SecretPaths) updateSecret(name string, dat []byte) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if len(dat) > 0 && dat[len(dat)-1] == 0xa {
		dat = dat[:len(dat)-1]
	}
	sp.secrets[name] = dat
}

// Add adds a file or directory to find secrets in all files
// found. The basename of the file will be the key to get the secret
func (sp *SecretPaths) Add(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		log.Errorf("Failed to stat path: %v", err)
		return err
	}

	switch mode := fi.Mode(); {
	case mode.IsRegular():
		return sp.registerSecretFile(fi.Name(), p)

	case mode.IsDir():
		return sp.handleDir(p)

	case mode&os.ModeSymlink != 0: // TODO(sszuecs) do we want to support symlinks or not?
		err := sp.registerSecretFile(fi.Name(), p) // link to regular file
		if err != nil {
			return sp.tryDir(p)
		}
		return nil
	}

	log.Errorf("File type not supported, only regular, directories and symlinks are supported")
	return ErrWrongFileType
}

func (sp *SecretPaths) handleDir(p string) error {
	m, err := filepath.Glob(p + "/*")
	if err != nil {
		return err
	}

	numErrors := 0
	for _, s := range m {
		if err = sp.registerSecretFile(filepath.Base(s), s); err != nil {
			numErrors += 1
		}
	}
	if numErrors == len(m) {
		return ErrFailedToReadFile
	}

	return nil
}

func (sp *SecretPaths) tryDir(p string) error {
	_, err := filepath.Glob(p + "/*")
	if err != nil {
		return ErrWrongFileType
	}
	return sp.handleDir(p)
}

func (sp *SecretPaths) registerSecretFile(name, p string) error {
	if _, ok := sp.GetSecret(name); ok {
		return ErrAlreadyExists
	}
	dat, err := ioutil.ReadFile(p)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", p, err)
		return err
	}
	sp.updateSecret(name, dat)
	sp.files[name] = p

	return nil
}

// runRefresher refreshes all secrets, that are registered
func (sp *SecretPaths) runRefresher(d time.Duration) {
	log.Infof("Run secrets path refresher every %s", d)
	for {
		select {
		case <-time.After(d):
			for name, p := range sp.files {
				dat, err := ioutil.ReadFile(p)
				if err != nil {
					log.Errorf("Failed to read file (%s): %v", p, err)
					continue
				}
				log.Infof("update secret file: %s", name)
				sp.updateSecret(name, dat)
			}
		case <-sp.quit:
			log.Infoln("Stop secrets background refresher")
			return
		}
	}
}

func (sp *SecretPaths) Close() {
	close(sp.quit)
}
