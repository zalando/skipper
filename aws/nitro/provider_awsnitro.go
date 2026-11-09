//go:build awsnitro && linux

package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/letsencrypt/pkcs11key/v4"
)

type acmAPI interface {
	GetCertificate(context.Context, *acm.GetCertificateInput, ...func(*acm.Options)) (*acm.GetCertificateOutput, error)
}

type signerPool interface {
	crypto.Signer
	Destroy() error
}

type provider struct {
	client      acmAPI
	config      Config
	newSigner   func(Config, crypto.PublicKey) (signerPool, error)
	certificate atomic.Pointer[tls.Certificate]
	mu          sync.Mutex
	pools       []signerPool
	closeOnce   sync.Once
	closeErr    error
	ctx         context.Context
	cancel      context.CancelFunc
	refreshDone chan struct{}
}

const refreshTimeout = 30 * time.Second

// New creates a provider, loads its first ACM certificate, and starts refreshes.
func New(config Config) (Provider, error) {
	if config.CertificateARN == "" {
		return nil, errors.New("certificate ARN is required")
	}
	if !filepath.IsAbs(config.ModulePath) {
		return nil, errors.New("PKCS#11 module path must be absolute")
	}
	if config.TokenLabel == "" {
		return nil, errors.New("PKCS#11 token label is required")
	}
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = DefaultRefreshInterval
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &provider{
		client:      acm.NewFromConfig(awsCfg),
		config:      config,
		newSigner:   newPKCS11Signer,
		ctx:         ctx,
		cancel:      cancel,
		refreshDone: make(chan struct{}),
	}
	refreshCtx, refreshCancel := context.WithTimeout(ctx, refreshTimeout)
	err = p.refresh(refreshCtx)
	refreshCancel()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load ACM certificate: %w", err)
	}
	go p.refreshLoop()
	return p, nil
}

func (p *provider) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	certificate := p.certificate.Load()
	if certificate == nil {
		return nil, errors.New("AWS Nitro certificate is not available")
	}
	return certificate, nil
}

func (p *provider) refreshLoop() {
	defer close(p.refreshDone)
	ticker := time.NewTicker(p.config.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(p.ctx, refreshTimeout)
			err := p.refresh(ctx)
			cancel()
			if err != nil {
				log.Printf("AWS Nitro certificate refresh failed: %v", err)
			}
		}
	}
}

func (p *provider) refresh(ctx context.Context) error {
	output, err := p.client.GetCertificate(ctx, &acm.GetCertificateInput{
		CertificateArn: aws.String(p.config.CertificateARN),
	})
	if err != nil {
		return fmt.Errorf("get ACM certificate: %w", err)
	}
	if output == nil || output.Certificate == nil {
		return errors.New("ACM returned no certificate")
	}
	chain, parsed, err := parseCertificateChain(*output.Certificate, output.CertificateChain)
	if err != nil {
		return err
	}
	if _, ok := parsed[0].PublicKey.(*rsa.PublicKey); !ok {
		return fmt.Errorf("AWS ACM for Nitro Enclaves requires an RSA certificate, got %T", parsed[0].PublicKey)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	var signer signerPool
	if previous := p.certificate.Load(); previous != nil && bytes.Equal(previous.Leaf.RawSubjectPublicKeyInfo, parsed[0].RawSubjectPublicKeyInfo) {
		signer, _ = previous.PrivateKey.(signerPool)
	}
	if signer == nil {
		signer, err = p.newSigner(p.config, parsed[0].PublicKey)
		if err != nil {
			return fmt.Errorf("initialize PKCS#11 signer: %w", err)
		}
		p.pools = append(p.pools, signer)
	}
	p.certificate.Store(&tls.Certificate{
		Certificate: chain,
		PrivateKey:  signer,
		Leaf:        parsed[0],
	})
	return nil
}

func newPKCS11Signer(config Config, publicKey crypto.PublicKey) (signerPool, error) {
	return pkcs11key.NewPool(1, config.ModulePath, config.TokenLabel, config.PIN, publicKey)
}

func parseCertificateChain(certificate string, chainPEM *string) ([][]byte, []*x509.Certificate, error) {
	contents := certificate
	if chainPEM != nil {
		contents += "\n" + *chainPEM
	}
	var (
		chain  [][]byte
		parsed []*x509.Certificate
	)
	for rest := []byte(contents); len(rest) > 0; {
		block, remaining := pem.Decode(rest)
		if block == nil {
			if strings.TrimSpace(string(rest)) != "" {
				return nil, nil, errors.New("ACM returned invalid PEM certificate data")
			}
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse ACM certificate: %w", err)
		}
		chain = append(chain, block.Bytes)
		parsed = append(parsed, certificate)
	}
	if len(parsed) == 0 {
		return nil, nil, errors.New("ACM returned an empty certificate chain")
	}
	return chain, parsed, nil
}

func (p *provider) Close() error {
	p.cancel()
	<-p.refreshDone
	p.closeOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		var errs []error
		for _, pool := range p.pools {
			if err := pool.Destroy(); err != nil {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
