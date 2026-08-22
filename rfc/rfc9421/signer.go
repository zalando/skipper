package rfc9421

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Supported cryptographic signature algorithms.
const (
	AlgHmacSha256   = "hmac-sha256"
	AlgRsaPssSha512 = "rsa-pss-sha512"
	AlgRsaV15Sha256 = "rsa-v1_5-sha256"
	AlgEcdsaP256    = "ecdsa-p256-sha256"
	AlgEd25519      = "ed25519"
)

// Signer orchestrates HTTP message signing according to RFC 9421.
type Signer struct {
	KeyID          string
	Algorithm      string
	Components     []string
	SignatureLabel string
	PrivateKey     []byte
	parsedKey      crypto.PrivateKey
}

// NewSigner validates parameters, parses the private key once, and initializes a Signer.
func NewSigner(keyID, algorithm string, components []string, sigLabel string, privateKey []byte) (*Signer, error) {
	if keyID == "" {
		return nil, errors.New("rfc9421: key ID cannot be empty")
	}
	if len(components) == 0 {
		return nil, errors.New("rfc9421: signature components cannot be empty")
	}
	if len(privateKey) == 0 {
		return nil, errors.New("rfc9421: private key cannot be empty")
	}

	normAlg := strings.ToLower(strings.TrimSpace(algorithm))
	switch normAlg {
	case AlgHmacSha256, AlgRsaPssSha512, AlgRsaV15Sha256, AlgEcdsaP256, AlgEd25519:
	default:
		return nil, fmt.Errorf("rfc9421: unsupported algorithm %q", algorithm)
	}

	label := strings.TrimSpace(sigLabel)
	if label == "" {
		label = "sig1"
	}

	parsedKey, err := parsePrivateKey(normAlg, privateKey)
	if err != nil {
		return nil, fmt.Errorf("rfc9421: failed to parse private key: %w", err)
	}

	return &Signer{
		KeyID:          keyID,
		Algorithm:      normAlg,
		Components:     components,
		SignatureLabel: label,
		PrivateKey:     privateKey,
		parsedKey:      parsedKey,
	}, nil
}

// SignRequest computes the RFC 9421 signature and adds Signature-Input and Signature headers.
func (s *Signer) SignRequest(req *http.Request) error {
	created := time.Now()

	sigInputParams := s.formatSignatureParams(created)
	sigInputHeader := fmt.Sprintf("%s=%s", s.SignatureLabel, sigInputParams)

	signatureBase, err := s.BuildSignatureBase(req, sigInputParams)
	if err != nil {
		return fmt.Errorf("rfc9421: failed to build signature base: %w", err)
	}

	rawSig, err := s.signData([]byte(signatureBase))
	if err != nil {
		return fmt.Errorf("rfc9421: failed to generate signature: %w", err)
	}

	sigHeader := fmt.Sprintf("%s=:%s:", s.SignatureLabel, base64.StdEncoding.EncodeToString(rawSig))

	req.Header.Set("Signature-Input", sigInputHeader)
	req.Header.Set("Signature", sigHeader)

	return nil
}

// formatSignatureParams constructs the RFC 8941 inner-list string using strings.Builder.
func (s *Signer) formatSignatureParams(created time.Time) string {
	var b strings.Builder
	b.WriteString("(")
	for i, comp := range s.Components {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString("\"")
		b.WriteString(strings.ToLower(strings.TrimSpace(comp)))
		b.WriteString("\"")
	}
	b.WriteString(");created=")
	b.WriteString(strconv.FormatInt(created.Unix(), 10))
	if s.KeyID != "" {
		b.WriteString(";keyid=\"")
		b.WriteString(s.KeyID)
		b.WriteString("\"")
	}
	if s.Algorithm != "" {
		b.WriteString(";alg=\"")
		b.WriteString(s.Algorithm)
		b.WriteString("\"")
	}
	return b.String()
}

// parsePrivateKey parses the raw bytes or PEM-encoded key for the chosen algorithm.
func parsePrivateKey(alg string, keyData []byte) (crypto.PrivateKey, error) {
	if alg == AlgHmacSha256 {
		return keyData, nil
	}

	block, _ := pem.Decode(keyData)
	data := keyData
	if block != nil {
		data = block.Bytes
	}

	if pk, err := x509.ParsePKCS8PrivateKey(data); err == nil {
		return pk, nil
	}

	switch alg {
	case AlgRsaPssSha512, AlgRsaV15Sha256:
		if pk, err := x509.ParsePKCS1PrivateKey(data); err == nil {
			return pk, nil
		}
	case AlgEcdsaP256:
		if pk, err := x509.ParseECPrivateKey(data); err == nil {
			return pk, nil
		}
	case AlgEd25519:
		if len(data) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(data), nil
		}
		if len(data) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(data), nil
		}
	}

	return nil, errors.New("invalid or unrecognized private key format")
}

// signData computes the cryptographic signature over the provided base bytes.
func (s *Signer) signData(data []byte) ([]byte, error) {
	switch s.Algorithm {
	case AlgHmacSha256:
		key, ok := s.parsedKey.([]byte)
		if !ok {
			return nil, errors.New("invalid HMAC key")
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(data)
		return mac.Sum(nil), nil

	case AlgEd25519:
		key, ok := s.parsedKey.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("invalid Ed25519 key")
		}
		return ed25519.Sign(key, data), nil

	case AlgRsaPssSha512:
		key, ok := s.parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("invalid RSA-PSS key")
		}
		hashed := sha512.Sum512(data)
		return rsa.SignPSS(rand.Reader, key, crypto.SHA512, hashed[:], &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
		})

	case AlgRsaV15Sha256:
		key, ok := s.parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("invalid RSA key")
		}
		hashed := sha256.Sum256(data)
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])

	case AlgEcdsaP256:
		key, ok := s.parsedKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("invalid ECDSA key")
		}
		hashed := sha256.Sum256(data)
		return ecdsa.SignASN1(rand.Reader, key, hashed[:])

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", s.Algorithm)
	}
}
