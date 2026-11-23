//go:build awsnitro

package nitro

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
)

// RFC 8017 Section 9.2 DigestInfo prefixes for RSASSA-PKCS1-v1_5.
var rsaDigestInfoPrefixes = map[crypto.Hash][]byte{
	crypto.SHA1:   {0x30, 0x21, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x14},
	crypto.SHA256: {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
	crypto.SHA384: {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
	crypto.SHA512: {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
}

type pooledSession struct {
	client    *p11Client
	sessionID uint64
}

func (ps *pooledSession) close() {
	if ps == nil || ps.client == nil {
		return
	}
	if ps.sessionID != 0 {
		_ = ps.client.closeSession(ps.sessionID)
	}
	_ = ps.client.close()
}

type sessionPool struct {
	mu     sync.Mutex
	closed bool
	idle   chan *pooledSession
	sem    chan struct{}
	dial   func(context.Context) (*p11Client, error)
	slotID uint64
}

func newSessionPool(maxSessions int, slotID uint64, dial func(context.Context) (*p11Client, error)) *sessionPool {
	return &sessionPool{
		idle:   make(chan *pooledSession, maxSessions),
		sem:    make(chan struct{}, maxSessions),
		dial:   dial,
		slotID: slotID,
	}
}

func (p *sessionPool) acquire(ctx context.Context) (*pooledSession, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("aws/nitro: session pool closed")
	}
	p.mu.Unlock()

	select {
	case ps := <-p.idle:
		return ps, nil
	default:
	}

	select {
	case ps := <-p.idle:
		return ps, nil
	case p.sem <- struct{}{}:
		ps, err := p.open(ctx)
		if err != nil {
			<-p.sem
			return nil, err
		}
		return ps, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *sessionPool) open(ctx context.Context) (*pooledSession, error) {
	client, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	sess, err := client.openSession(p.slotID)
	if err != nil {
		_ = client.close()
		return nil, err
	}
	return &pooledSession{
		client:    client,
		sessionID: sess,
	}, nil
}

func (p *sessionPool) release(ps *pooledSession, broken bool) {
	if ps == nil {
		return
	}
	if broken {
		ps.close()
		<-p.sem
		return
	}

	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		ps.close()
		<-p.sem
		return
	}

	select {
	case p.idle <- ps:
	default:
		ps.close()
		<-p.sem
	}
}

func (p *sessionPool) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	for {
		select {
		case ps := <-p.idle:
			ps.close()
			<-p.sem
		default:
			return
		}
	}
}

type enclaveSigner struct {
	pub       crypto.PublicKey
	keyID     []byte
	keyHandle uint64
	pool      *sessionPool
}

var _ crypto.Signer = (*enclaveSigner)(nil)

func (s *enclaveSigner) Public() crypto.PublicKey {
	return s.pub
}

func (s *enclaveSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	mech, payload, isECDSA, err := s.prepareSign(digest, opts)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := range 2 {
		ps, err := s.pool.acquire(context.Background())
		if err != nil {
			return nil, err
		}

		keyHandle := s.keyHandle
		if attempt > 0 && len(s.keyID) > 0 {
			if refreshed, findErr := findPrivateKeyByID(ps.client, ps.sessionID, s.keyID); findErr == nil {
				keyHandle = refreshed
			}
		}

		sig, err := ps.client.sign(ps.sessionID, keyHandle, mech, payload)
		if err != nil {
			var ckErr CKError
			broken := !errors.As(err, &ckErr) ||
				uint64(ckErr) == ckrSessionClosed ||
				uint64(ckErr) == ckrSessionHandleInvalid
			s.pool.release(ps, broken)
			lastErr = err
			if broken && attempt == 0 {
				continue
			}
			return nil, err
		}
		s.pool.release(ps, false)

		if isECDSA {
			return rawECDSAToASN1(sig)
		}
		return sig, nil
	}
	return nil, lastErr
}

func (s *enclaveSigner) prepareSign(digest []byte, opts crypto.SignerOpts) (p11Mechanism, []byte, bool, error) {
	switch s.pub.(type) {
	case *ecdsa.PublicKey:
		return p11Mechanism{typ: ckmECDSA}, digest, true, nil
	case *rsa.PublicKey:
		if pssOpts, ok := opts.(*rsa.PSSOptions); ok {
			params, err := buildPSSParams(pssOpts, len(digest))
			if err != nil {
				return p11Mechanism{}, nil, false, err
			}
			return p11Mechanism{typ: ckmRSAPKCSPSS, pssParams: params}, digest, false, nil
		}
		h := opts.HashFunc()
		prefix, ok := rsaDigestInfoPrefixes[h]
		if !ok {
			return p11Mechanism{}, nil, false, fmt.Errorf("aws/nitro: unsupported RSA hash %v", h)
		}
		if len(digest) != h.Size() {
			return p11Mechanism{}, nil, false, fmt.Errorf("aws/nitro: invalid digest length %d for %v", len(digest), h)
		}
		payload := make([]byte, 0, len(prefix)+len(digest))
		payload = append(payload, prefix...)
		payload = append(payload, digest...)
		return p11Mechanism{typ: ckmRSAPKCS}, payload, false, nil
	default:
		return p11Mechanism{}, nil, false, fmt.Errorf("aws/nitro: unsupported public key type %T", s.pub)
	}
}

func buildPSSParams(opts *rsa.PSSOptions, digestLen int) (*p11PSSParams, error) {
	var hashAlg, mgf uint64
	switch opts.HashFunc() {
	case crypto.SHA1:
		hashAlg, mgf = ckmSHA1, ckgMGF1SHA1
	case crypto.SHA256:
		hashAlg, mgf = ckmSHA256, ckgMGF1SHA256
	case crypto.SHA384:
		hashAlg, mgf = ckmSHA384, ckgMGF1SHA384
	case crypto.SHA512:
		hashAlg, mgf = ckmSHA512, ckgMGF1SHA512
	default:
		return nil, fmt.Errorf("aws/nitro: unsupported RSA-PSS hash %v", opts.HashFunc())
	}

	saltLen := opts.SaltLength
	switch saltLen {
	case rsa.PSSSaltLengthAuto, rsa.PSSSaltLengthEqualsHash:
		saltLen = digestLen
	}
	if saltLen < 0 {
		return nil, fmt.Errorf("aws/nitro: invalid RSA-PSS salt length %d", saltLen)
	}

	return &p11PSSParams{
		hashAlg: hashAlg,
		mgf:     mgf,
		saltLen: uint64(saltLen),
	}, nil
}

// PKCS#11 CKM_ECDSA returns raw concatenated big-endian r || s, whereas
// crypto/tls expects an ASN.1 DER Ecdsa-Sig-Value sequence (RFC 3279).
func rawECDSAToASN1(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return nil, fmt.Errorf("aws/nitro: invalid raw ECDSA signature length %d", len(raw))
	}
	half := len(raw) / 2
	sig := struct {
		R, S *big.Int
	}{
		R: new(big.Int).SetBytes(raw[:half]),
		S: new(big.Int).SetBytes(raw[half:]),
	}
	return asn1.Marshal(sig)
}
