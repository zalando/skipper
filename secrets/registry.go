package secrets

import "time"

type EncrypterCreator interface {
	NewEncrypter(time.Duration, string) (Encryption, error)
}

type Encryption interface {
	CreateNonce() ([]byte, error)
	Decrypt([]byte) ([]byte, error)
	Encrypt([]byte) ([]byte, error)
	Close()
}

type Registry struct {
	encrypterMap map[string]*Encrypter
}

// NewRegistry returns a Registry and implements EncrypterCreator to
// store and manage secrets
func NewRegistry() *Registry {
	e := make(map[string]*Encrypter)
	return &Registry{
		encrypterMap: e,
	}
}

func (r *Registry) NewEncrypter(refreshInterval time.Duration, file string) (Encryption, error) {
	if e, ok := r.encrypterMap[file]; ok {
		return e, nil
	}

	e, err := newEncrypter(file)
	if err != nil {
		return nil, err
	}

	if refreshInterval > 0 {
		err := e.runCipherRefresher(refreshInterval)
		if err != nil {
			return nil, err
		}

	}
	r.encrypterMap[file] = e
	return e, nil
}

// Close will close all Encryption of the Registry
func (r *Registry) Close() {
	for _, v := range r.encrypterMap {
		v.Close()
	}
}
