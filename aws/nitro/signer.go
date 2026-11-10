//go:build awsnitro

package nitro

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"
)

var hashPrefixes = map[crypto.Hash][]byte{
	crypto.SHA1:   {0x30, 0x21, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x14},
	crypto.SHA224: {0x30, 0x2d, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x04, 0x05, 0x00, 0x04, 0x1c},
	crypto.SHA256: {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
	crypto.SHA384: {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
	crypto.SHA512: {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
}

type (
	ecdsaSigValue struct {
		R, S *big.Int
	}

	nitroSigner struct {
		client    *p11RPCClient
		slotID    uint64
		keyHandle uint64
		pin       string
		pub       crypto.PublicKey
		sessions  chan uint64
	}
)

func newNitroSigner(client *p11RPCClient, slotID, keyHandle uint64, pin string, pub crypto.PublicKey, poolSize int) *nitroSigner {
	if poolSize <= 0 {
		poolSize = 32
	}
	return &nitroSigner{
		client:    client,
		slotID:    slotID,
		keyHandle: keyHandle,
		pin:       pin,
		pub:       pub,
		sessions:  make(chan uint64, poolSize),
	}
}

func (s *nitroSigner) Public() crypto.PublicKey {
	return s.pub
}

func (s *nitroSigner) acquireSession() (uint64, error) {
	select {
	case h := <-s.sessions:
		return h, nil
	default:
	}

	h, err := s.client.openSession(s.slotID)
	if err != nil {
		return 0, err
	}
	if s.pin != "" {
		if err := s.client.login(h, s.pin); err != nil {
			_ = s.client.closeSession(h)
			return 0, err
		}
	}
	return h, nil
}

func (s *nitroSigner) releaseSession(h uint64) {
	select {
	case s.sessions <- h:
	default:
		_ = s.client.closeSession(h)
	}
}

func (s *nitroSigner) closeAllSessions() {
	for {
		select {
		case h := <-s.sessions:
			_ = s.client.closeSession(h)
		default:
			return
		}
	}
}

func (s *nitroSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	sess, err := s.acquireSession()
	if err != nil {
		return nil, err
	}

	switch pub := s.pub.(type) {
	case *rsa.PublicKey:
		sig, err := s.signRSA(sess, pub, digest, opts)
		if err != nil {
			_ = s.client.closeSession(sess)
			return nil, err
		}
		s.releaseSession(sess)
		return sig, nil
	case *ecdsa.PublicKey:
		sig, err := s.signECDSA(sess, pub, digest)
		if err != nil {
			_ = s.client.closeSession(sess)
			return nil, err
		}
		s.releaseSession(sess)
		return sig, nil
	default:
		_ = s.client.closeSession(sess)
		return nil, fmt.Errorf("unsupported public key type: %T", s.pub)
	}
}

func (s *nitroSigner) signRSA(sess uint64, pub *rsa.PublicKey, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	maxSigLen := uint32(pub.Size())
	if pssOpts, ok := opts.(*rsa.PSSOptions); ok {
		pssParams, err := buildPSSParams(pssOpts.Hash, pssOpts.SaltLength)
		if err != nil {
			return nil, err
		}
		return s.client.sign(sess, s.keyHandle, p11Mechanism{
			Type:      ckmRSAPKCSPSS,
			PSSParams: pssParams,
		}, digest, maxSigLen)
	}

	hashFunc := crypto.Hash(0)
	if opts != nil {
		hashFunc = opts.HashFunc()
	}
	var payload []byte
	if hashFunc == 0 {
		payload = digest
	} else {
		prefix, ok := hashPrefixes[hashFunc]
		if !ok {
			return nil, fmt.Errorf("unsupported RSA hash function: %v", hashFunc)
		}
		payload = make([]byte, len(prefix)+len(digest))
		copy(payload, prefix)
		copy(payload[len(prefix):], digest)
	}
	return s.client.sign(sess, s.keyHandle, p11Mechanism{Type: ckmRSAPKCS}, payload, maxSigLen)
}

func (s *nitroSigner) signECDSA(sess uint64, pub *ecdsa.PublicKey, digest []byte) ([]byte, error) {
	byteLen := (pub.Curve.Params().BitSize + 7) / 8
	rawSig, err := s.client.sign(sess, s.keyHandle, p11Mechanism{Type: ckmECDSA}, digest, uint32(byteLen*2))
	if err != nil {
		return nil, err
	}
	if len(rawSig) == 0 || len(rawSig)%2 != 0 {
		return nil, fmt.Errorf("invalid ECDSA signature length: %d", len(rawSig))
	}
	half := len(rawSig) / 2
	r := new(big.Int).SetBytes(rawSig[:half])
	sVal := new(big.Int).SetBytes(rawSig[half:])
	return asn1.Marshal(ecdsaSigValue{R: r, S: sVal})
}

func buildPSSParams(hash crypto.Hash, saltLen int) (*p11RSAPSSParams, error) {
	var hashAlg, mgf, hLen uint64
	switch hash {
	case crypto.SHA1:
		hashAlg, mgf, hLen = ckmSHA1, ckgMGF1SHA1, 20
	case crypto.SHA224:
		hashAlg, mgf, hLen = ckmSHA224, ckgMGF1SHA224, 28
	case crypto.SHA256:
		hashAlg, mgf, hLen = ckmSHA256, ckgMGF1SHA256, 32
	case crypto.SHA384:
		hashAlg, mgf, hLen = ckmSHA384, ckgMGF1SHA384, 48
	case crypto.SHA512:
		hashAlg, mgf, hLen = ckmSHA512, ckgMGF1SHA512, 64
	default:
		return nil, fmt.Errorf("unsupported PSS hash: %v", hash)
	}

	sLen := hLen
	if saltLen > 0 {
		sLen = uint64(saltLen)
	}
	return &p11RSAPSSParams{
		HashAlg: hashAlg,
		MGF:     mgf,
		SLen:    sLen,
	}, nil
}
