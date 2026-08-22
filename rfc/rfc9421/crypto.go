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
	"encoding/base64"
	"fmt"
)

// signData computes the cryptographic signature over the provided data
// using the algorithm and private key configured on the Signer.
func (s *Signer) signData(data []byte) (string, error) {
	var sigBytes []byte
	var err error

	switch s.config.Algorithm {
	case AlgorithmRsaPssSha512:
		sigBytes, err = s.signRsaPssSha512(data)
	case AlgorithmRsaV15Sha256:
		sigBytes, err = s.signRsaV15Sha256(data)
	case AlgorithmHmacSha256:
		sigBytes, err = s.signHmacSha256(data)
	case AlgorithmEcdsaP256:
		sigBytes, err = s.signEcdsaP256(data)
	case AlgorithmEd25519:
		sigBytes, err = s.signEd25519(data)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, s.config.Algorithm)
	}

	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(sigBytes), nil
}

func (s *Signer) signRsaPssSha512(data []byte) ([]byte, error) {
	privKey, ok := s.config.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected *rsa.PrivateKey", ErrInvalidKeyType)
	}

	hashed := sha512.Sum512(data)
	opts := &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
		Hash:       crypto.SHA512,
	}

	return rsa.SignPSS(rand.Reader, privKey, crypto.SHA512, hashed[:], opts)
}

func (s *Signer) signRsaV15Sha256(data []byte) ([]byte, error) {
	privKey, ok := s.config.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected *rsa.PrivateKey", ErrInvalidKeyType)
	}

	hashed := sha256.Sum256(data)
	return rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hashed[:])
}

func (s *Signer) signHmacSha256(data []byte) ([]byte, error) {
	var keyBytes []byte

	switch k := s.config.PrivateKey.(type) {
	case []byte:
		keyBytes = k
	case string:
		keyBytes = []byte(k)
	default:
		return nil, fmt.Errorf("%w: expected []byte or string for HMAC", ErrInvalidKeyType)
	}

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write(data)
	return mac.Sum(nil), nil
}

func (s *Signer) signEcdsaP256(data []byte) ([]byte, error) {
	privKey, ok := s.config.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected *ecdsa.PrivateKey", ErrInvalidKeyType)
	}

	hashed := sha256.Sum256(data)
	r, sVal, err := ecdsa.Sign(rand.Reader, privKey, hashed[:])
	if err != nil {
		return nil, err
	}

	// RFC 9421 Section 3.2.2: ECDSA P-256 signature is concatenation of r and s (32 bytes each)
	rBytes := r.Bytes()
	sBytes := sVal.Bytes()

	out := make([]byte, 64)
	copy(out[32-len(rBytes):32], rBytes)
	copy(out[64-len(sBytes):64], sBytes)

	return out, nil
}

func (s *Signer) signEd25519(data []byte) ([]byte, error) {
	privKey, ok := s.config.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected ed25519.PrivateKey", ErrInvalidKeyType)
	}

	return ed25519.Sign(privKey, data), nil
}
