package secrets

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"gopkg.in/fsnotify.v1"
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
	watcher *fsnotify.Watcher
	known   map[string][]byte
}

func NewSecretPaths() *SecretPaths {
	sp := &SecretPaths{
		known: make(map[string][]byte),
	}

	if watcher, err := fsnotify.NewWatcher(); err != nil {
		log.Errorf("Failed to create watcher, probably no support for fsnotify: %v", err)
	} else {
		sp.watcher = watcher
		go sp.startRefresher()
	}

	return sp
}

// GetSecret returns secret and if found or not for a given name.
func (sp *SecretPaths) GetSecret(s string) ([]byte, bool) {
	dat, ok := sp.known[s]
	return dat, ok
}

func (sp *SecretPaths) updateSecret(name string, dat []byte) {
	if len(dat) > 0 && dat[len(dat)-1] == 0xa {
		dat = dat[:len(dat)-1]
	}
	sp.known[name] = dat
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
	if _, ok := sp.known[name]; ok {
		return ErrAlreadyExists
	}
	dat, err := ioutil.ReadFile(p)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", p, err)
		return err
	}
	sp.updateSecret(name, dat)

	err = sp.registerWatcher(p)
	if err != nil {
		log.Errorf("Failed to watch path '%s': %v", p, err)
	}
	return nil
}

func (sp *SecretPaths) registerWatcher(p string) error {
	if sp.watcher == nil {
		return nil
	}
	return sp.watcher.Add(p)
}

// startRefresher refreshes all secrets, that are registered on the watcher
func (sp *SecretPaths) startRefresher() {
	for {
		select {
		case event, ok := <-sp.watcher.Events:
			if !ok {
				log.Infoln("Stop secrets background refresher")
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				dat, err := ioutil.ReadFile(event.Name)
				if err != nil {
					log.Errorf("Failed to read file (%s): %v", event.Name, err)
					continue
				}
				log.Infof("update secret file: %s", event.Name)
				sp.updateSecret(filepath.Base(event.Name), dat)
			}
		case err, ok := <-sp.watcher.Errors:
			if !ok {
				log.Infoln("Stop secrets background refresher")
				return
			}
			log.Errorf("Watch event error: %v", err)
		}
	}
}

func (sp *SecretPaths) Close() {
	log.Info("Stop secret updates")
	sp.watcher.Close()
}
