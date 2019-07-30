package secrets

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/syncmap"
)

const (
	defaultCredentialsUpdateInterval = 10 * time.Minute
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
	mu              sync.RWMutex
	quit            chan struct{}
	secrets         map[string][]byte
	files           *syncmap.Map
	refreshInterval time.Duration
	started         bool
}

// NewSecretPaths creates a SecretPaths, that implements a
// SecretsProvider. It runs every d interval background refresher as a
// side effect. On tear down make sure to Close() it.
func NewSecretPaths(d time.Duration) *SecretPaths {
	if d <= 0 {
		d = defaultCredentialsUpdateInterval
	}

	return &SecretPaths{
		quit:            make(chan struct{}),
		secrets:         make(map[string][]byte),
		files:           &syncmap.Map{},
		refreshInterval: d,
		started:         false,
	}
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
// found. The basename of the file will be the key to get the
// secret. Add is not synchronized and is not safe to call
// concurrently. Add has a side effect of lazily init a goroutine to
// start a single background refresher for the SecretPaths instance.
func (sp *SecretPaths) Add(p string) error {
	if !sp.started {
		// lazy init background goroutine, such that we have only a goroutine if there is work
		go sp.runRefresher()
		sp.started = true
	}

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
	sp.files.Store(name, p)

	return nil
}

// runRefresher refreshes all secrets, that are registered
func (sp *SecretPaths) runRefresher() {
	log.Infof("Run secrets path refresher every %s", sp.refreshInterval)
	for {
		select {
		case <-time.After(sp.refreshInterval):
			sp.files.Range(func(k, b interface{}) bool {
				name, ok := k.(string)
				if !ok {
					log.Errorf("Failed to convert k '%v' to string", k)
					return true
				}
				p, ok := b.(string)
				if !ok {
					log.Errorf("Failed to convert p '%v' to string", b)
					return true
				}
				dat, err := ioutil.ReadFile(p)
				if err != nil {
					log.Errorf("Failed to read file (%s): %v", p, err)
					return true
				}
				log.Infof("update secret file: %s", name)
				sp.updateSecret(name, dat)
				return true
			})
		case <-sp.quit:
			log.Infoln("Stop secrets background refresher")
			return
		}
	}
}

func (sp *SecretPaths) Close() {
	close(sp.quit)
}
