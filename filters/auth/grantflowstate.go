package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zalando/skipper/secrets"
)

type state struct {
	Validity   int64  `json:"validity"`
	Nonce      string `json:"nonce"`
	RequestURL string `json:"redirectUrl"`
}

type flowState struct {
	secrets     *secrets.Registry
	secretsFile string
}

var errExpiredAuthState = errors.New("expired auth state")

func newFlowState(secrets *secrets.Registry, secretsFile string) *flowState {
	return &flowState{
		secrets:     secrets,
		secretsFile: secretsFile,
	}
}

func stateValidityTime() int64 {
	return time.Now().Add(time.Hour).Unix()
}

func (s *flowState) createState(redirectURL string) (string, error) {
	encrypter, err := s.secrets.GetEncrypter(secretsRefreshInternal, s.secretsFile)
	if err != nil {
		return "", err
	}

	nonce, err := encrypter.CreateNonce()
	if err != nil {
		return "", err
	}

	state := state{
		Validity:   stateValidityTime(),
		Nonce:      fmt.Sprintf("%x", nonce),
		RequestURL: redirectURL,
	}

	jb, err := json.Marshal(state)
	if err != nil {
		return "", err
	}

	eb, err := encrypter.Encrypt(jb)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", eb), nil
}

func (s *flowState) extractState(st string) (state state, err error) {
	var encrypter secrets.Encryption
	if encrypter, err = s.secrets.GetEncrypter(secretsRefreshInternal, s.secretsFile); err != nil {
		return
	}

	var eb []byte
	if _, err = fmt.Sscanf(st, "%x", &eb); err != nil {
		return
	}

	var jb []byte
	if jb, err = encrypter.Decrypt(eb); err != nil {
		return
	}

	if err = json.Unmarshal(jb, &state); err != nil {
		return
	}

	validity := time.Unix(state.Validity, 0)
	if validity.Before(time.Now()) {
		err = errExpiredAuthState
	}

	return
}

func (s *flowState) Close() {
	s.secrets.Close()
}
