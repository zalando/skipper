package auth

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/rfc/rfc9421"
	"github.com/zalando/skipper/secrets"
)

type httpMessageSignatureSpec struct {
	keyFile       string
	keyID         string
	secretsReader secrets.SecretsReader
}

type httpMessageSignatureFilter struct {
	keyFile       string
	keyID         string
	secretsReader secrets.SecretsReader
	algorithm     string
	components    []string
	label         string

	mu        sync.RWMutex
	cachedKey []byte
	signer    *rfc9421.Signer
}

// NewHTTPMessageSignature creates a new HTTP Message Signatures filter specification (RFC 9421).
func NewHTTPMessageSignature(keyFile, keyID string, secretsReader secrets.SecretsReader) filters.Spec {
	return &httpMessageSignatureSpec{
		keyFile:       keyFile,
		keyID:         keyID,
		secretsReader: secretsReader,
	}
}

func (s *httpMessageSignatureSpec) Name() string {
	return filters.HTTPMessageSignatureName
}

func (s *httpMessageSignatureSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	if s.keyFile == "" || s.keyID == "" || s.secretsReader == nil {
		return nil, fmt.Errorf("httpMessageSignature: key file, key ID, and secrets reader must be configured: %w", filters.ErrInvalidFilterParameters)
	}

	if len(config) < 2 || len(config) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	algorithm, ok := config[0].(string)
	if !ok || algorithm == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	componentsStr, ok := config[1].(string)
	if !ok || componentsStr == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	rawComponents := strings.Split(componentsStr, ",")
	components := make([]string, 0, len(rawComponents))
	for _, c := range rawComponents {
		c = strings.TrimSpace(c)
		if c != "" {
			components = append(components, c)
		}
	}
	if len(components) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	label := "sig1"
	if len(config) == 3 {
		l, ok := config[2].(string)
		if !ok || l == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		label = l
	}

	f := &httpMessageSignatureFilter{
		keyFile:       s.keyFile,
		keyID:         s.keyID,
		secretsReader: s.secretsReader,
		algorithm:     algorithm,
		components:    components,
		label:         label,
	}

	keyBytes, ok := s.secretsReader.GetSecret(s.keyFile)
	if ok && len(keyBytes) > 0 {
		_, _ = f.getSigner(keyBytes)
	}

	return f, nil
}

func (f *httpMessageSignatureFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req == nil {
		return
	}

	keyBytes, ok := f.secretsReader.GetSecret(f.keyFile)
	if !ok || len(keyBytes) == 0 {
		return
	}

	signer, err := f.getSigner(keyBytes)
	if err != nil {
		return
	}

	_ = signer.SignRequest(req)
}

func (f *httpMessageSignatureFilter) Response(filters.FilterContext) {}

func (f *httpMessageSignatureFilter) getSigner(keyBytes []byte) (*rfc9421.Signer, error) {
	f.mu.RLock()
	if f.signer != nil && bytes.Equal(f.cachedKey, keyBytes) {
		s := f.signer
		f.mu.RUnlock()
		return s, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double check after acquiring write lock
	if f.signer != nil && bytes.Equal(f.cachedKey, keyBytes) {
		return f.signer, nil
	}

	signer, err := rfc9421.NewSigner(f.keyID, f.algorithm, f.components, f.label, keyBytes)
	if err != nil {
		return nil, err
	}

	f.cachedKey = bytes.Clone(keyBytes)
	f.signer = signer
	return signer, nil
}
