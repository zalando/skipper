//go:build awsnitro

package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/zalando/skipper/metrics"
)

type (
	session struct {
		conn   *rpcConn
		handle uint64
		key    uint64
	}

	signer struct {
		public  crypto.PublicKey
		cfg     Certificate
		pin     []byte
		dial    func(context.Context) (net.Conn, error)
		timeout time.Duration
		metrics metrics.Metrics
		// slots limits the number of concurrent sessions.
		slots  chan struct{}
		quit   chan struct{}
		mu     sync.Mutex
		idle   []*session
		closed bool
	}
)

var (
	_ crypto.Signer = (*signer)(nil)

	errSignerClosed = errors.New("aws nitro signer closed")
)

// DigestInfo prefixes for PKCS #1 v1.5 signatures, see RFC 8017
// section 9.2 and crypto/rsa/pkcs1v15.go.
var pkcs1Prefix = map[crypto.Hash][]byte{
	crypto.SHA1:    {0x30, 0x21, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x14},
	crypto.SHA224:  {0x30, 0x2d, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x04, 0x05, 0x00, 0x04, 0x1c},
	crypto.SHA256:  {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
	crypto.SHA384:  {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
	crypto.SHA512:  {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
	crypto.MD5SHA1: {},
}

var pssHash = map[crypto.Hash][2]uint64{
	crypto.SHA1:   {ckmSHA1, ckgMGF1SHA1},
	crypto.SHA224: {ckmSHA224, ckgMGF1SHA224},
	crypto.SHA256: {ckmSHA256, ckgMGF1SHA256},
	crypto.SHA384: {ckmSHA384, ckgMGF1SHA384},
	crypto.SHA512: {ckmSHA512, ckgMGF1SHA512},
}

func newSigner(o Options, cfg Certificate, pin []byte, public crypto.PublicKey) (*signer, error) {
	switch public.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
	default:
		return nil, fmt.Errorf("unsupported public key type %T", public)
	}

	s := &signer{
		public:  public,
		cfg:     cfg,
		pin:     pin,
		dial:    o.dial,
		timeout: o.Timeout,
		metrics: o.Metrics,
		slots:   make(chan struct{}, o.SessionsPerToken),
		quit:    make(chan struct{}),
	}

	if err := s.verify(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// verify checks that the enclave key matches the public key of the leaf certificate.
func (s *signer) verify() error {
	digest := crypto.SHA256.New()
	digest.Write([]byte("skipper aws nitro key verification"))
	sum := digest.Sum(nil)

	sig, err := s.Sign(nil, sum, crypto.SHA256)
	if err != nil {
		return err
	}

	switch pub := s.public.(type) {
	case *rsa.PublicKey:
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum, sig)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, sum, sig) {
			err = errors.New("ecdsa verification failed")
		}
	}
	if err != nil {
		return fmt.Errorf("key of token %q does not match certificate %s: %w", s.cfg.TokenLabel, s.cfg.ChainPath, err)
	}
	return nil
}

func (s *signer) Public() crypto.PublicKey {
	return s.public
}

// Sign implements crypto.Signer. The random source is ignored, the
// enclave provides randomness for RSA-PSS and ECDSA.
func (s *signer) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	start := time.Now()
	sig, err := s.sign(digest, opts)
	if err != nil {
		s.metrics.IncCounter("awsnitro.sign.error")
		return nil, err
	}
	s.metrics.MeasureSince("awsnitro.sign", start)
	return sig, nil
}

func (s *signer) sign(digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	h := opts.HashFunc()
	if h != 0 && h != crypto.MD5SHA1 && len(digest) != h.Size() {
		return nil, fmt.Errorf("digest length %d does not match hash %v", len(digest), h)
	}

	var (
		m    mechanism
		data []byte
	)
	switch pub := s.public.(type) {
	case *rsa.PublicKey:
		if pss, ok := opts.(*rsa.PSSOptions); ok {
			p, ok := pssHash[h]
			if !ok {
				return nil, fmt.Errorf("unsupported hash for rsa pss: %v", h)
			}
			saltLen := pss.SaltLength
			switch saltLen {
			case rsa.PSSSaltLengthEqualsHash:
				saltLen = h.Size()
			case rsa.PSSSaltLengthAuto:
				saltLen = (pub.N.BitLen()-1+7)/8 - 2 - h.Size()
			}
			if saltLen < 0 {
				return nil, fmt.Errorf("invalid rsa pss salt length %d", pss.SaltLength)
			}
			m = mechanism{typ: ckmRSAPKCSPSS, pss: &pssParams{hashAlg: p[0], mgf: p[1], sLen: uint64(saltLen)}}
			data = digest
		} else {
			prefix, ok := pkcs1Prefix[h]
			if !ok {
				return nil, fmt.Errorf("unsupported hash for rsa pkcs1v15: %v", h)
			}
			m = mechanism{typ: ckmRSAPKCS}
			data = append(bytes.Clone(prefix), digest...)
		}
	case *ecdsa.PublicKey:
		m = mechanism{typ: ckmECDSA}
		data = digest
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	var sig []byte
	for {
		sess, fresh, err := s.get(ctx)
		if err != nil {
			return nil, err
		}
		sig, err = sess.conn.sign(ctx, sess.handle, sess.key, m, data)
		s.put(sess, err)
		if err == nil {
			break
		}
		// idle connections may be broken, for example after an enclave restart
		if fresh || isCKError(err) || ctx.Err() != nil {
			return nil, fmt.Errorf("failed to sign with token %q: %w", s.cfg.TokenLabel, err)
		}
	}

	if pub, ok := s.public.(*ecdsa.PublicKey); ok {
		return ecdsaASN1(pub, sig)
	}
	return sig, nil
}

// ecdsaASN1 converts the PKCS#11 r || s signature to ASN.1 as expected
// by crypto/tls, see PKCS#11 v2.40 section 2.3.1.
func ecdsaASN1(pub *ecdsa.PublicKey, sig []byte) ([]byte, error) {
	n := (pub.Curve.Params().BitSize + 7) / 8
	if len(sig) != 2*n {
		return nil, fmt.Errorf("invalid ecdsa signature length %d, expected %d", len(sig), 2*n)
	}
	return asn1.Marshal(struct{ R, S *big.Int }{
		new(big.Int).SetBytes(sig[:n]),
		new(big.Int).SetBytes(sig[n:]),
	})
}

// get returns an idle session or opens a new one. The returned bool
// reports whether the session was opened by this call.
func (s *signer) get(ctx context.Context) (*session, bool, error) {
	select {
	case <-s.quit:
		return nil, false, errSignerClosed
	case <-ctx.Done():
		return nil, false, fmt.Errorf("no enclave session available for token %q: %w", s.cfg.TokenLabel, ctx.Err())
	case s.slots <- struct{}{}:
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.slots
		return nil, false, errSignerClosed
	}
	if n := len(s.idle); n > 0 {
		sess := s.idle[n-1]
		s.idle = s.idle[:n-1]
		s.mu.Unlock()
		return sess, false, nil
	}
	s.mu.Unlock()

	sess, err := s.open(ctx)
	if err != nil {
		<-s.slots
		return nil, false, err
	}
	return sess, true, nil
}

// put returns the session to the pool. Sessions with transport or
// protocol errors are closed and reopened on next use.
func (s *signer) put(sess *session, err error) {
	defer func() { <-s.slots }()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || err != nil && !isCKError(err) || isSessionError(err) {
		sess.conn.Close()
		return
	}
	s.idle = append(s.idle, sess)
}

func isSessionError(err error) bool {
	return errors.Is(err, ckError(ckrSessionHandleInvalid)) ||
		errors.Is(err, ckError(ckrKeyHandleInvalid)) ||
		errors.Is(err, ckError(ckrObjectHandleInvalid)) ||
		errors.Is(err, ckError(ckrUserNotLoggedIn)) ||
		errors.Is(err, ckError(ckrCryptokiNotInitialized)) ||
		errors.Is(err, ckError(ckrDeviceError))
}

func (s *signer) open(ctx context.Context) (*session, error) {
	s.metrics.IncCounter("awsnitro.connect")
	conn, err := newRPCConn(ctx, s.dial, s.timeout)
	if err != nil {
		s.metrics.IncCounter("awsnitro.connect.error")
		return nil, fmt.Errorf("failed to connect to aws nitro enclave: %w", err)
	}
	sess, err := s.openSession(ctx, conn)
	if err != nil {
		s.metrics.IncCounter("awsnitro.connect.error")
		conn.Close()
		return nil, err
	}
	return sess, nil
}

func (s *signer) openSession(ctx context.Context, conn *rpcConn) (*session, error) {
	if err := conn.initialize(ctx); err != nil {
		return nil, err
	}
	slots, err := conn.slotList(ctx)
	if err != nil {
		return nil, err
	}

	var (
		slot  uint64
		ti    tokenInfo
		found bool
	)
	for _, sl := range slots {
		ti, err = conn.tokenInfo(ctx, sl)
		if err != nil {
			return nil, err
		}
		if ti.label == s.cfg.TokenLabel {
			slot, found = sl, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("token %q not found in aws nitro enclave", s.cfg.TokenLabel)
	}

	handle, err := conn.openSession(ctx, slot)
	if err != nil {
		return nil, err
	}
	if len(s.pin) > 0 {
		err = conn.login(ctx, handle, s.pin)
	} else if ti.flags&ckfLoginRequired != 0 {
		err = fmt.Errorf("token %q requires login, but no pin is configured", s.cfg.TokenLabel)
	}
	if err != nil {
		conn.closeSession(ctx, handle)
		return nil, err
	}

	template := []attribute{ulongAttribute(ckaClass, ckoPrivateKey)}
	if s.cfg.KeyLabel != "" {
		template = append(template, bytesAttribute(ckaLabel, []byte(s.cfg.KeyLabel)))
	}
	keys, err := conn.findObjects(ctx, handle, template)
	if err == nil && len(keys) != 1 {
		err = fmt.Errorf("found %d private keys in token %q, expected exactly one, use key=<label> to select one", len(keys), s.cfg.TokenLabel)
	}
	if err != nil {
		conn.closeSession(ctx, handle)
		return nil, err
	}

	return &session{conn: conn, handle: handle, key: keys[0]}, nil
}

// Close closes idle sessions. Sessions in use are closed when returned.
func (s *signer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.quit)
	idle := s.idle
	s.idle = nil
	s.mu.Unlock()

	for _, sess := range idle {
		s.closeSession(sess)
	}
	return nil
}

func (s *signer) closeSession(sess *session) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	sess.conn.closeSession(ctx, sess.handle)
	sess.conn.finalize(ctx)
	sess.conn.Close()
}
