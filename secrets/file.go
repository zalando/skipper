package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	defaultCredentialsUpdateInterval = 10 * time.Minute
)

var (
	ErrWrongFileType    = errors.New("file type not supported")
	ErrFailedToReadFile = errors.New("failed to read file")
	errEmptyFile        = errors.New("empty file")
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

type secretMap map[string][]byte

type SecretPaths struct {
	// See https://pkg.go.dev/sync/atomic#example-Value-ReadMostly
	secrets atomic.Value // secretMap
	closed  sync.Once
	quit    chan struct{}
	mu      sync.Mutex
	paths   map[string]struct{}
}

// NewSecretPaths creates a SecretPaths, that implements a
// SecretsProvider. It runs every d interval background refresher as a
// side effect. On tear down make sure to Close() it.
func NewSecretPaths(d time.Duration) *SecretPaths {
	if d <= 0 {
		d = defaultCredentialsUpdateInterval
	}

	sp := &SecretPaths{
		quit:  make(chan struct{}),
		paths: make(map[string]struct{}),
	}
	sp.secrets.Store(make(secretMap))

	go sp.runRefresher(d)

	return sp
}

// GetSecret returns secret and if found or not for a given name.
func (sp *SecretPaths) GetSecret(s string) ([]byte, bool) {
	m := sp.secrets.Load().(secretMap)
	data, ok := m[s]
	return data, ok
}

// Add registers path to a file or directory to find secrets.
// Background refresher discovers files added or removed later to the directory path.
// The path of the file will be the key to get the secret.
func (sp *SecretPaths) Add(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}

	switch mode := fi.Mode(); {
	// Kubernetes uses symlink to file
	case mode.IsRegular() || mode&os.ModeSymlink != 0:
		if _, err := readSecretFile(path); err != nil {
			return err
		}
	case mode.IsDir():
		// handled by refresh
	default:
		return ErrWrongFileType
	}

	sp.mu.Lock()
	sp.paths[path] = struct{}{}
	sp.refreshLocked()
	sp.mu.Unlock()

	return nil
}

// runRefresher periodically refreshes all registered paths
func (sp *SecretPaths) runRefresher(d time.Duration) {
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.mu.Lock()
			sp.refreshLocked()
			sp.mu.Unlock()
		case <-sp.quit:
			log.Infoln("Stop secrets background refresher")
			return
		}
	}
}

// refreshLocked reads secrets from all registered paths and updates secrets map.
// sp.mu must be held
func (sp *SecretPaths) refreshLocked() {
	sizeHint := len(sp.secrets.Load().(secretMap))

	actual := make(secretMap, sizeHint)
	for path := range sp.paths {
		addPath(actual, path)
	}

	old := sp.secrets.Swap(actual).(secretMap)

	for path, data := range actual {
		oldData, existed := old[path]
		if !existed {
			log.Infof("Added secret file: %s", path)
		} else if !bytes.Equal(data, oldData) {
			log.Infof("Updated secret file: %s", path)
		}
	}

	for path := range old {
		if _, exists := actual[path]; !exists {
			log.Infof("Removed secret file: %s", path)
		}
	}
}

func addPath(secrets secretMap, path string) {
	fi, err := os.Lstat(path)
	if err != nil {
		log.Errorf("Failed to stat path %s: %v", path, err)
		return
	}

	switch mode := fi.Mode(); {
	// Kubernetes uses symlink to file
	case mode.IsRegular() || mode&os.ModeSymlink != 0:
		data, err := readSecretFile(path)
		if err != nil {
			log.Errorf("Failed to read file %s: %v", path, err)
			return
		}
		secrets[path] = data
	case mode.IsDir():
		matches, err := filepath.Glob(path + "/*")
		if err != nil {
			log.Errorf("Failed to read directory %s: %v", path, err)
			return
		}
		for _, match := range matches {
			data, err := readSecretFile(match)
			if err == nil {
				secrets[match] = data
			} else if !errors.Is(err, syscall.EISDIR) {
				log.Errorf("Failed to read path %s: %v", match, err)
			}
		}
	}
}

func readSecretFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 && data[len(data)-1] == 0xa {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return nil, errEmptyFile
	}
	return data, nil
}

func (sp *SecretPaths) Close() {
	sp.closed.Do(func() {
		close(sp.quit)
	})
}
