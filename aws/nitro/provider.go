//go:build awsnitro

package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

type certSnapshot struct {
	exact     map[string]*tls.Certificate
	wildcards map[string]*tls.Certificate
	fallback  *tls.Certificate
	pool      *sessionPool
}

// Provider supplies TLS certificates backed by an AWS Nitro Enclave running
// AWS ACM for Nitro Enclaves.
type Provider struct {
	opts     Options
	endpoint parsedEndpoint
	snap     atomic.Pointer[certSnapshot]

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// New connects to the AWS Nitro Enclave p11-kit RPC endpoint, loads available
// ACM certificates and private key handles, and starts a background refresh
// loop for certificate renewals.
func New(opts Options) (*Provider, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	ep, err := parseEndpoint(opts.Endpoint)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Provider{
		opts:     opts,
		endpoint: ep,
		cancel:   cancel,
	}

	if err := p.refresh(ctx); err != nil {
		cancel()
		return nil, err
	}

	p.wg.Add(1)
	go p.refreshLoop(ctx)

	return p, nil
}

// GetCertificate selects the matching TLS certificate for the client's SNI
// server name, supporting exact hostnames, single-level wildcards, and a
// fallback certificate when SNI is omitted.
func (p *Provider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	snap := p.snap.Load()
	if snap == nil {
		return nil, ErrNoCertificates
	}

	name := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
	if name == "" {
		if snap.fallback != nil {
			return snap.fallback, nil
		}
		return nil, ErrNoCertificates
	}

	if cert, ok := snap.exact[name]; ok {
		return cert, nil
	}

	if _, rest, ok := strings.Cut(name, "."); ok && rest != "" {
		if cert, ok := snap.wildcards["*."+rest]; ok {
			return cert, nil
		}
	}

	if snap.fallback != nil {
		return snap.fallback, nil
	}
	return nil, fmt.Errorf("aws/nitro: no certificate matching %q", hello.ServerName)
}

// Close stops background certificate refresh and closes all pooled enclave
// sessions.
func (p *Provider) Close() error {
	p.closeOnce.Do(func() {
		p.cancel()
		p.wg.Wait()
		if snap := p.snap.Swap(nil); snap != nil && snap.pool != nil {
			snap.pool.close()
		}
	})
	return nil
}

func (p *Provider) refreshLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.opts.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.refresh(ctx); err != nil {
				log.Errorf("aws/nitro: certificate refresh failed: %v", err)
			}
		}
	}
}

func (p *Provider) dialClient(ctx context.Context) (*p11Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()

	conn, err := dialEndpoint(dialCtx, p.endpoint)
	if err != nil {
		return nil, err
	}
	rc, err := newRPCConn(conn, p.opts.Timeout)
	if err != nil {
		return nil, err
	}
	client, err := newP11Client(rc)
	if err != nil {
		_ = rc.close()
		return nil, err
	}
	return client, nil
}

func (p *Provider) refresh(ctx context.Context) error {
	client, err := p.dialClient(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.close() }()

	slotID, err := p.selectSlot(client)
	if err != nil {
		return err
	}

	sess, err := client.openSession(slotID)
	if err != nil {
		return fmt.Errorf("aws/nitro: open session: %w", err)
	}
	defer func() { _ = client.closeSession(sess) }()

	certs, err := loadCertificates(client, sess)
	if err != nil {
		return err
	}
	if len(certs) == 0 {
		return ErrNoCertificates
	}

	pool := newSessionPool(p.opts.MaxSessions, slotID, p.dialClient)
	snap := buildSnapshot(certs, pool)
	if snap.fallback == nil {
		pool.close()
		return ErrNoCertificates
	}

	old := p.snap.Swap(snap)
	if old != nil && old.pool != nil {
		old.pool.close()
	}
	return nil
}

func (p *Provider) selectSlot(client *p11Client) (uint64, error) {
	slots, err := client.getSlotList(true)
	if err != nil {
		return 0, fmt.Errorf("aws/nitro: get slot list: %w", err)
	}
	if len(slots) == 0 {
		return 0, errors.New("aws/nitro: no PKCS#11 slots with token present")
	}
	if p.opts.TokenLabel == "" {
		return slots[0], nil
	}

	for _, slotID := range slots {
		label, err := client.getTokenLabel(slotID)
		if err != nil {
			continue
		}
		if label == p.opts.TokenLabel {
			return slotID, nil
		}
	}
	return 0, fmt.Errorf("aws/nitro: token label %q not found", p.opts.TokenLabel)
}

type discoveredCert struct {
	leaf      *x509.Certificate
	chainDER  [][]byte
	keyID     []byte
	keyHandle uint64
}

func loadCertificates(client *p11Client, sess uint64) ([]discoveredCert, error) {
	certHandles, err := client.findObjects(sess, []p11Attribute{
		attrUlong(ckaClass, ckoCertificate),
		attrUlong(ckaCertificateType, ckcX509),
	})
	if err != nil {
		return nil, fmt.Errorf("aws/nitro: find certificates: %w", err)
	}

	type rawCert struct {
		parsed *x509.Certificate
		id     []byte
	}
	var allCerts []rawCert

	for _, h := range certHandles {
		attrs, err := client.getAttributes(sess, h, []p11AttrRequest{
			{typ: ckaValue, bufLen: 65536},
			{typ: ckaID, bufLen: 1024},
		})
		if err != nil {
			continue
		}
		if !attrs[0].valid || len(attrs[0].bytes) == 0 {
			continue
		}
		parsed, err := x509.ParseCertificate(attrs[0].bytes)
		if err != nil {
			continue
		}
		var id []byte
		if attrs[1].valid {
			id = attrs[1].bytes
		}
		allCerts = append(allCerts, rawCert{parsed: parsed, id: id})
	}

	var result []discoveredCert
	for _, rc := range allCerts {
		if rc.parsed.IsCA || len(rc.id) == 0 {
			continue
		}
		if !isSupportedPublicKey(rc.parsed.PublicKey) {
			continue
		}
		keyHandle, err := findPrivateKeyByID(client, sess, rc.id)
		if err != nil {
			continue
		}

		chain := [][]byte{rc.parsed.Raw}
		for _, cand := range allCerts {
			if bytes.Equal(cand.parsed.Raw, rc.parsed.Raw) {
				continue
			}
			if cand.parsed.IsCA && bytes.Equal(cand.id, rc.id) {
				chain = append(chain, cand.parsed.Raw)
			}
		}

		result = append(result, discoveredCert{
			leaf:      rc.parsed,
			chainDER:  chain,
			keyID:     rc.id,
			keyHandle: keyHandle,
		})
	}
	return result, nil
}

func findPrivateKeyByID(client *p11Client, sess uint64, id []byte) (uint64, error) {
	handles, err := client.findObjects(sess, []p11Attribute{
		attrUlong(ckaClass, ckoPrivateKey),
		attrBytes(ckaID, id),
	})
	if err != nil {
		return 0, err
	}
	if len(handles) == 0 {
		return 0, errors.New("aws/nitro: private key not found")
	}
	return handles[0], nil
}

func isSupportedPublicKey(pub crypto.PublicKey) bool {
	switch pub.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
		return true
	default:
		return false
	}
}

func buildSnapshot(certs []discoveredCert, pool *sessionPool) *certSnapshot {
	snap := &certSnapshot{
		exact:     make(map[string]*tls.Certificate),
		wildcards: make(map[string]*tls.Certificate),
		pool:      pool,
	}

	for _, dc := range certs {
		tlsCert := &tls.Certificate{
			Certificate: dc.chainDER,
			Leaf:        dc.leaf,
			PrivateKey: &enclaveSigner{
				pub:       dc.leaf.PublicKey,
				keyID:     dc.keyID,
				keyHandle: dc.keyHandle,
				pool:      pool,
			},
		}

		if snap.fallback == nil || dc.leaf.NotBefore.After(snap.fallback.Leaf.NotBefore) {
			snap.fallback = tlsCert
		}

		names := dc.leaf.DNSNames
		if len(names) == 0 && dc.leaf.Subject.CommonName != "" {
			names = []string{dc.leaf.Subject.CommonName}
		}
		for _, name := range names {
			n := strings.ToLower(strings.TrimSuffix(name, "."))
			if strings.HasPrefix(n, "*.") {
				if prev, ok := snap.wildcards[n]; !ok || dc.leaf.NotBefore.After(prev.Leaf.NotBefore) {
					snap.wildcards[n] = tlsCert
				}
			} else {
				if prev, ok := snap.exact[n]; !ok || dc.leaf.NotBefore.After(prev.Leaf.NotBefore) {
					snap.exact[n] = tlsCert
				}
			}
		}
	}
	return snap
}
