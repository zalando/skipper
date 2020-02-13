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
	secrets         *syncmap.Map
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
		secrets:         &syncmap.Map{},
		refreshInterval: d,
		started:         false,
	}
}

// GetSecret returns secret and if found or not for a given name.
func (sp *SecretPaths) GetSecret(s string) ([]byte, bool) {
	dat, ok := sp.secrets.Load(s)
	if !ok {
		return nil, false
	}
	b, ok := dat.([]byte)
	return b, ok
}

func (sp *SecretPaths) updateSecret(s string, dat []byte) {
	if len(dat) > 0 && dat[len(dat)-1] == 0xa {
		dat = dat[:len(dat)-1]
	}
	sp.secrets.Store(s, dat)
}

// Add adds a file or directory to find secrets in all files
// found. The basename of the file will be the key to get the
// secret. Add is not synchronized and is not safe to call
// concurrently. Add has a side effect of lazily init a goroutine to
// start a single background refresher for the SecretPaths instance.
func (sp *SecretPaths) Add(p string) error {
	sp.mu.Lock()
	if !sp.started {
		// lazy init background goroutine, such that we have only a goroutine if there is work
		go sp.runRefresher()
		sp.started = true
	}
	sp.mu.Unlock()

	fi, err := os.Lstat(p)
	if err != nil {
		log.Errorf("Failed to stat path: %v", err)
		return err
	}

	switch mode := fi.Mode(); {
	case mode.IsRegular():
		return sp.registerSecretFile(p)

	case mode.IsDir():
		return sp.handleDir(p)

	case mode&os.ModeSymlink != 0: // Kubernetes use symlink to file
		return sp.registerSecretFile(p)
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
		if err = sp.registerSecretFile(s); err != nil {
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

func (sp *SecretPaths) registerSecretFile(p string) error {
	if _, ok := sp.GetSecret(p); ok {
		return ErrAlreadyExists
	}
	dat, err := ioutil.ReadFile(p)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", p, err)
		return err
	}
	sp.updateSecret(p, dat)
	return nil
}

// runRefresher refreshes all secrets, that are registered
func (sp *SecretPaths) runRefresher() {
	log.Infof("Run secrets path refresher every %s, but update once first", sp.refreshInterval)
	var d time.Duration
	for {
		select {
		case <-time.After(d):
			sp.secrets.Range(func(k, _ interface{}) bool {
				f, ok := k.(string)
				if !ok {
					log.Errorf("Failed to convert k '%v' to string", k)
					return true
				}
				sec, err := ioutil.ReadFile(f)
				if err != nil {
					log.Errorf("Failed to read file (%s): %v", f, err)
					return true
				}
				log.Infof("update secret file: %s", f)
				sp.updateSecret(f, sec)
				return true
			})
		case <-sp.quit:
			log.Infoln("Stop secrets background refresher")
			return
		}
		d = sp.refreshInterval
	}
}

func (sp *SecretPaths) Close() {
	if sp != nil {
		close(sp.quit)
	}
}
