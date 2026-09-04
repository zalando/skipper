package auth

import (
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/rfc/rfc9421"
	"github.com/zalando/skipper/secrets"
)

const (
	// RFC9421FilterName is the filter name used in eskip routes.
	RFC9421FilterName = "rfc9421"
)

type rfc9421Spec struct {
	keyFile       string
	keyID         string
	secretsReader secrets.SecretsReader
}

type rfc9421Filter struct {
	signer *rfc9421.Signer
}

// NewRFC9421 creates a new filter specification for RFC 9421 HTTP Message Signatures.
func NewRFC9421(keyFile, keyID string, secretsReader secrets.SecretsReader) filters.Spec {
	return &rfc9421Spec{
		keyFile:       keyFile,
		keyID:         keyID,
		secretsReader: secretsReader,
	}
}

// Name returns the eskip filter name.
func (s *rfc9421Spec) Name() string {
	return RFC9421FilterName
}

// CreateFilter creates an instance of the RFC 9421 signing filter.
// Args expected:
//
//	[0] algorithm (string): cryptographic signing algorithm (e.g. "hmac-sha256", "ed25519", "rsa-pss-sha512")
//	[1] components (string): comma-separated HTTP components to cover (e.g. "@method, @path, @authority")
//	[2] signatureLabel (string, optional): signature label prefix (default: "sig1")
func (s *rfc9421Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if s.keyFile == "" || s.keyID == "" {
		return nil, fmt.Errorf("rfc9421: key file and key ID must be configured via flags: %w", filters.ErrInvalidFilterParameters)
	}

	algorithm, ok := args[0].(string)
	if !ok || strings.TrimSpace(algorithm) == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	componentsStr, ok := args[1].(string)
	if !ok || strings.TrimSpace(componentsStr) == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	label := "sig1"
	if len(args) == 3 {
		customLabel, ok := args[2].(string)
		if !ok || strings.TrimSpace(customLabel) == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		label = strings.TrimSpace(customLabel)
	}

	var keyBytes []byte
	if s.secretsReader != nil {
		if kb, ok := s.secretsReader.GetSecret(s.keyFile); ok && len(kb) > 0 {
			keyBytes = kb
		}
	}

	if len(keyBytes) == 0 {
		var err error
		keyBytes, err = os.ReadFile(s.keyFile)
		if err != nil {
			return nil, fmt.Errorf("rfc9421: failed to read key file %q: %w", s.keyFile, err)
		}
	}

	if len(keyBytes) == 0 {
		return nil, fmt.Errorf("rfc9421: secret %q is empty", s.keyFile)
	}

	var components []string
	for part := range strings.SplitSeq(componentsStr, ",") {
		c := strings.TrimSpace(part)
		if c != "" {
			components = append(components, c)
		}
	}

	signer, err := rfc9421.NewSigner(s.keyID, algorithm, components, label, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("rfc9421: failed to initialize signer: %w", err)
	}

	return &rfc9421Filter{
		signer: signer,
	}, nil
}

// Request signs the outgoing request.
func (f *rfc9421Filter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if err := f.signer.SignRequest(req); err != nil {
		log.Errorf("rfc9421: failed to sign request: %v", err)
	}
}

// Response is a no-op for RFC 9421 request signing.
func (f *rfc9421Filter) Response(ctx filters.FilterContext) {}
