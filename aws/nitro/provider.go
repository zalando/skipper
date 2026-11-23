//go:build awsnitro

package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/secrets/certregistry"
)

// retiredSignerGracePeriod delays closing the signer of a replaced
// certificate, such that in-flight handshakes can finish.
const retiredSignerGracePeriod = time.Minute

type (
	token struct {
		cfg    Certificate
		chain  []byte
		signer *signer
	}

	// Provider loads certificates with private keys in an AWS Nitro
	// Enclave into a certregistry.CertRegistry and keeps them up to date.
	Provider struct {
		opts     Options
		pin      []byte
		registry *certregistry.CertRegistry
		tokens   []*token
		quit     chan struct{}
		done     chan struct{}
		once     sync.Once
		mu       sync.Mutex
		retired  []*signer
		timers   []*time.Timer
	}
)

// NewProvider connects to the enclave, verifies that the private key of
// every configured token matches its certificate chain and registers
// the certificates in the registry for all their host names.
func NewProvider(o Options, registry *certregistry.CertRegistry) (*Provider, error) {
	if registry == nil {
		return nil, errors.New("aws nitro requires a certificate registry")
	}
	if len(o.Certificates) == 0 {
		return nil, errors.New("aws nitro requires at least one certificate")
	}
	if o.dial == nil {
		if o.CID == 0 || o.Port == 0 {
			return nil, errors.New("aws nitro requires vsock cid and port")
		}
		cid, port := o.CID, o.Port
		o.dial = func(ctx context.Context) (net.Conn, error) { return dialVsock(ctx, cid, port) }
	}
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = DefaultRefreshInterval
	}
	if o.SessionsPerToken <= 0 {
		o.SessionsPerToken = DefaultSessionsPerToken
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Metrics == nil {
		o.Metrics = metrics.Default
	}

	p := &Provider{
		opts:     o,
		registry: registry,
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	if o.PINFile != "" {
		pin, err := os.ReadFile(o.PINFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read aws nitro pin file: %w", err)
		}
		p.pin = bytes.TrimSpace(pin)
	}

	for _, cfg := range o.Certificates {
		t := &token{cfg: cfg}
		if err := p.load(t); err != nil {
			p.closeTokens()
			return nil, err
		}
		p.tokens = append(p.tokens, t)
	}

	go p.refresh()
	return p, nil
}

func (p *Provider) load(t *token) error {
	chain, err := os.ReadFile(t.cfg.ChainPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate chain: %w", err)
	}
	if t.signer != nil && bytes.Equal(chain, t.chain) {
		return nil
	}

	cert, err := parseChain(chain)
	if err != nil {
		return fmt.Errorf("failed to parse certificate chain %s: %w", t.cfg.ChainPath, err)
	}

	s := t.signer
	if s == nil || !samePublicKey(s.public, cert.Leaf.PublicKey) {
		s, err = newSigner(p.opts, t.cfg, p.pin, cert.Leaf.PublicKey)
		if err != nil {
			return err
		}
	}
	cert.PrivateKey = s

	for _, host := range hosts(cert.Leaf) {
		if err := p.registry.ConfigureCertificateWithPriority(host, cert, certregistry.PriorityNitro); err != nil {
			if s != t.signer {
				s.Close()
			}
			return fmt.Errorf("failed to register certificate for %s: %w", host, err)
		}
	}
	log.Infof("aws nitro certificate loaded for token %q: %s", t.cfg.TokenLabel, strings.Join(hosts(cert.Leaf), ","))

	if t.signer != nil && t.signer != s {
		p.retire(t.signer)
	}
	t.chain = chain
	t.signer = s
	return nil
}

func samePublicKey(a, b crypto.PublicKey) bool {
	k, ok := a.(interface{ Equal(crypto.PublicKey) bool })
	return ok && k.Equal(b)
}

func (p *Provider) retire(s *signer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retired = append(p.retired, s)
	p.timers = append(p.timers, time.AfterFunc(retiredSignerGracePeriod, func() { s.Close() }))
}

func (p *Provider) refresh() {
	defer close(p.done)
	ticker := time.NewTicker(p.opts.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.quit:
			return
		case <-ticker.C:
			for _, t := range p.tokens {
				if err := p.load(t); err != nil {
					p.opts.Metrics.IncCounter("awsnitro.refresh.error")
					log.Errorf("failed to refresh aws nitro certificate for token %q: %v", t.cfg.TokenLabel, err)
				}
			}
		}
	}
}

func (p *Provider) closeTokens() {
	for _, t := range p.tokens {
		t.signer.Close()
	}
}

// Close stops refreshing certificates and closes all enclave sessions.
func (p *Provider) Close() error {
	p.once.Do(func() {
		close(p.quit)
		<-p.done
		p.closeTokens()

		p.mu.Lock()
		defer p.mu.Unlock()
		for _, t := range p.timers {
			t.Stop()
		}
		for _, s := range p.retired {
			s.Close()
		}
	})
	return nil
}

func parseChain(data []byte) (*tls.Certificate, error) {
	cert := &tls.Certificate{}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert.Certificate = append(cert.Certificate, block.Bytes)
		}
	}
	if len(cert.Certificate) == 0 {
		return nil, errors.New("no certificate found")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	cert.Leaf = leaf
	return cert, nil
}

func hosts(leaf *x509.Certificate) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, h := range append([]string{leaf.Subject.CommonName}, leaf.DNSNames...) {
		h = strings.ToLower(h)
		if _, ok := seen[h]; ok || h == "" {
			continue
		}
		seen[h] = struct{}{}
		result = append(result, h)
	}
	return result
}
