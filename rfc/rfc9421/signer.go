package rfc9421

import (
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Supported signing algorithms under RFC 9421.
const (
	AlgorithmRsaPssSha512 = "rsa-pss-sha512"
	AlgorithmRsaV15Sha256 = "rsa-v1_5-sha256"
	AlgorithmHmacSha256   = "hmac-sha256"
	AlgorithmEcdsaP256    = "ecdsa-p256-sha256"
	AlgorithmEd25519      = "ed25519"
)

var (
	ErrUnsupportedAlgorithm = errors.New("unsupported signature algorithm")
	ErrMissingKey           = errors.New("missing private key for signing")
	ErrMissingComponent     = errors.New("required component missing from request")
	ErrInvalidKeyType       = errors.New("invalid key type for selected algorithm")
)

// SignerConfig holds parameters required to generate an RFC 9421 signature.
type SignerConfig struct {
	KeyID      string
	Algorithm  string
	PrivateKey crypto.PrivateKey
	Components []string
	SignLabel  string
	Created    *time.Time
	Expires    *time.Time
	Nonce      string
}

// Signer orchestrates HTTP message signing according to RFC 9421.
type Signer struct {
	config SignerConfig
}

// NewSigner validates configuration and initializes an RFC 9421 Signer.
func NewSigner(config SignerConfig) (*Signer, error) {
	if config.PrivateKey == nil {
		return nil, ErrMissingKey
	}

	if config.SignLabel == "" {
		config.SignLabel = "sig1"
	}

	switch config.Algorithm {
	case AlgorithmRsaPssSha512, AlgorithmRsaV15Sha256, AlgorithmHmacSha256, AlgorithmEcdsaP256, AlgorithmEd25519:
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, config.Algorithm)
	}

	return &Signer{config: config}, nil
}

// SignRequest constructs the signature base string, hashes and signs it,
// and returns the values for the Signature-Input and Signature headers.
func (s *Signer) SignRequest(req *http.Request) (signatureInputHeader string, signatureHeader string, err error) {
	createdTime := time.Now()
	if s.config.Created != nil {
		createdTime = *s.config.Created
	}

	signatureParams := s.buildSignatureParams(createdTime)
	baseString, err := s.buildSignatureBase(req, signatureParams)
	if err != nil {
		return "", "", err
	}

	rawSignature, err := s.signData([]byte(baseString))
	if err != nil {
		return "", "", err
	}

	signatureInputHeader = fmt.Sprintf("%s=%s", s.config.SignLabel, signatureParams)
	signatureHeader = fmt.Sprintf("%s=:%s:", s.config.SignLabel, rawSignature)

	return signatureInputHeader, signatureHeader, nil
}

// buildSignatureParams formats the signature parameters according to RFC 9421 Section 2.3.
func (s *Signer) buildSignatureParams(created time.Time) string {
	var compStrings []string
	for _, comp := range s.config.Components {
		clean := strings.TrimSpace(comp)
		if clean != "" {
			compStrings = append(compStrings, fmt.Sprintf("%q", clean))
		}
	}

	params := fmt.Sprintf("(%s);created=%d", strings.Join(compStrings, " "), created.Unix())

	if s.config.KeyID != "" {
		params += fmt.Sprintf(";keyid=%q", s.config.KeyID)
	}

	if s.config.Algorithm != "" {
		params += fmt.Sprintf(";alg=%q", s.config.Algorithm)
	}

	if s.config.Expires != nil {
		params += fmt.Sprintf(";expires=%d", s.config.Expires.Unix())
	}

	if s.config.Nonce != "" {
		params += fmt.Sprintf(";nonce=%q", s.config.Nonce)
	}

	return params
}
