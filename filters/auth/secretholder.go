package auth

import (
	"io/ioutil"
	"strings"
)

//SecretSource operates on the secret for OpenID
type SecretSource interface {
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

func NewFileSecretSource(file string) SecretSource {
	return &FileSecretSource{fileName: file}
}
