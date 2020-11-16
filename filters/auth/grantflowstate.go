package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/zalando/skipper/secrets"
)

const randomStringLength = 20

type state struct {
	Rand       string `json:"rand"`
	Validity   int64  `json:"validity"`
	Nonce      string `json:"nonce"`
	RequestURL string `json:"redirectUrl"`
}

type flowState struct {
	secrets     *secrets.Registry
	secretsFile string
}

var errExpiredAuthState = errors.New("expired auth state")

const (
	secretSize    = 20
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var (
	src = rand.NewSource(time.Now().UnixNano())
)

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
func randString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

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
		Rand:       randString(randomStringLength),
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
