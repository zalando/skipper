package secrettest

import (
	"time"

	"github.com/zalando/skipper/secrets"
)

type TestRegistry struct {
	encrypterMap map[string]secrets.Encryption
}

var tr *TestRegistry

// NewTestRegistry returns a singleton TestRegistry
func NewTestRegistry() *TestRegistry {
	if tr != nil {
		return tr
	}
	e := make(map[string]secrets.Encryption)
	return &TestRegistry{
		encrypterMap: e,
	}
}

func (tr *TestRegistry) GetEncrypter(refreshInterval time.Duration, s string) (secrets.Encryption, error) {
	if e, ok := tr.encrypterMap[s]; ok {
		return e, nil
	}

	testEnc, err := secrets.WithSource(&TestingSecretSource{secretKey: s})
	if err != nil {
		return nil, err
	}

	if err := testEnc.RefreshCiphers(); err != nil {
		return nil, err
	}

	tr.encrypterMap[s] = testEnc
	return testEnc, nil
}

type TestingSecretSource struct {
	getCount  int
	secretKey string
}

func (s *TestingSecretSource) GetSecret() ([][]byte, error) {
	s.getCount++
	return [][]byte{[]byte(s.secretKey)}, nil
}
