//go:build awsnitro && linux

package nitro

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/secrets/certregistry"
)

// Provider manages certificates and TLS signing via an AWS Nitro Enclave.
type Provider struct {
	opts    Options
	cr      *certregistry.CertRegistry
	quit    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	client  *p11RPCClient
	cid     uint32
	port    uint32
	signers []*nitroSigner
}

// NewProvider initializes a Nitro Enclave TLS certificate provider, populates the
// certificate registry, and starts a background goroutine to refresh certificates.
func NewProvider(opts Options, cr *certregistry.CertRegistry) (*Provider, error) {
	if cr == nil {
		return nil, errors.New("certregistry cannot be nil")
	}
	if opts.EnclavePort == 0 {
		opts.EnclavePort = DefaultEnclavePort
	}
	if opts.ModulePath == "" {
		opts.ModulePath = DefaultModulePath
	}
	if opts.ConfPath == "" {
		opts.ConfPath = DefaultConfPath
	}
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = DefaultRefreshInterval
	}
	if opts.Dial == nil {
		opts.Dial = dialVsock
	}

	p := &Provider{
		opts: opts,
		cr:   cr,
		quit: make(chan struct{}),
	}

	if err := p.sync(); err != nil {
		return nil, err
	}

	p.wg.Add(1)
	go p.refreshLoop()

	return p, nil
}

// Close stops background certificate polling and closes all enclave sessions and connections.
func (p *Provider) Close() error {
	close(p.quit)
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.signers {
		s.closeAllSessions()
	}
	p.signers = nil
	if p.client != nil {
		err := p.client.Close()
		p.client = nil
		return err
	}
	return nil
}

func (p *Provider) refreshLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.opts.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.quit:
			return
		case <-ticker.C:
			if err := p.sync(); err != nil {
				log.Errorf("aws nitro tls sync failed: %v", err)
			}
		}
	}
}

func (p *Provider) resolveEndpoint() (uint32, uint32, error) {
	if p.opts.EnclaveCID != 0 {
		return p.opts.EnclaveCID, p.opts.EnclavePort, nil
	}
	cid, port, err := parseModuleFile(p.opts.ModulePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to discover enclave endpoint from %s: %w", p.opts.ModulePath, err)
	}
	if p.opts.EnclavePort != 0 && p.opts.EnclavePort != DefaultEnclavePort {
		port = p.opts.EnclavePort
	}
	return cid, port, nil
}

func (p *Provider) ensureClient(cid, port uint32) (*p11RPCClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil && !p.client.closed.Load() && p.cid == cid && p.port == port {
		return p.client, nil
	}

	if p.client != nil {
		for _, s := range p.signers {
			s.closeAllSessions()
		}
		p.signers = nil
		_ = p.client.Close()
		p.client = nil
	}

	conn, err := p.opts.Dial(cid, port)
	if err != nil {
		return nil, err
	}
	client, err := newP11RPCClient(conn)
	if err != nil {
		return nil, err
	}
	p.client = client
	p.cid = cid
	p.port = port
	return client, nil
}

func (p *Provider) resetClient() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.signers {
		s.closeAllSessions()
	}
	p.signers = nil
	if p.client != nil {
		_ = p.client.Close()
		p.client = nil
	}
}

func (p *Provider) sync() error {
	cid, port, err := p.resolveEndpoint()
	if err != nil {
		return err
	}

	client, err := p.ensureClient(cid, port)
	if err != nil {
		return err
	}

	slots, err := client.getSlotList(true)
	if err != nil {
		p.resetClient()
		return fmt.Errorf("failed to list enclave pkcs11 slots: %w", err)
	}
	if len(slots) == 0 {
		p.resetClient()
		return errors.New("no active pkcs11 tokens found in nitro enclave")
	}

	stanzas := parseStanzaFiles(p.opts.ConfPath)
	configured := 0

	for _, slotID := range slots {
		tokInfo, err := client.getTokenInfo(slotID)
		if err != nil {
			log.Warnf("failed to get token info for slot %d: %v", slotID, err)
			continue
		}

		pin := p.opts.PIN
		var certPath string
		if st, ok := stanzas[tokInfo.Label]; ok {
			if pin == "" {
				pin = st.PIN
			}
			certPath = st.CertPath
		} else if pin == "" && len(stanzas) == 1 {
			for _, st := range stanzas {
				pin = st.PIN
				certPath = st.CertPath
			}
		}

		if err := p.syncSlot(client, slotID, tokInfo, pin, certPath); err != nil {
			log.Warnf("failed to sync slot %d (token %q): %v", slotID, tokInfo.Label, err)
			continue
		}
		configured++
	}

	if configured == 0 {
		p.resetClient()
		return errors.New("failed to configure any certificate from nitro enclave")
	}
	return nil
}

func (p *Provider) syncSlot(client *p11RPCClient, slotID uint64, tokInfo *p11TokenInfo, pin, certPath string) error {
	sess, err := client.openSession(slotID)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.closeSession(sess)
	}()

	if (tokInfo.Flags&ckfLoginRequired != 0 || pin != "") && pin != "" {
		if err := client.login(sess, pin); err != nil {
			return fmt.Errorf("pkcs11 login failed: %w", err)
		}
	}

	certDERs, err := loadTokenCertificates(client, sess, certPath)
	if err != nil {
		return err
	}
	if len(certDERs) == 0 {
		return errors.New("no certificates found for token")
	}

	leaf, err := x509.ParseCertificate(certDERs[0])
	if err != nil {
		return fmt.Errorf("failed to parse leaf certificate: %w", err)
	}

	privObjs, err := client.findObjects(sess, []p11Attribute{
		{Type: ckaClass, ULong: ckoPrivateKey, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to find private key object: %w", err)
	}
	if len(privObjs) == 0 {
		return errors.New("no private key object found in token")
	}

	keyHandle := privObjs[0]
	signer := newNitroSigner(client, slotID, keyHandle, pin, leaf.PublicKey, 32)

	p.mu.Lock()
	p.signers = append(p.signers, signer)
	p.mu.Unlock()

	tlsCert := &tls.Certificate{
		Certificate: certDERs,
		PrivateKey:  signer,
		Leaf:        leaf,
	}

	hosts := make(map[string]struct{})
	for _, dns := range leaf.DNSNames {
		if dns != "" {
			hosts[dns] = struct{}{}
		}
	}
	if leaf.Subject.CommonName != "" {
		hosts[leaf.Subject.CommonName] = struct{}{}
	}
	if len(hosts) == 0 {
		return errors.New("certificate has no DNS SANs or CommonName")
	}

	for h := range hosts {
		if err := p.cr.ConfigureCertificate(h, tlsCert); err != nil {
			return err
		}
	}
	return nil
}

func loadTokenCertificates(client *p11RPCClient, sess uint64, certPath string) ([][]byte, error) {
	certObjs, err := client.findObjects(sess, []p11Attribute{
		{Type: ckaClass, ULong: ckoCertificate, Valid: true},
	})
	if err == nil && len(certObjs) > 0 {
		type certEntry struct {
			id       byte
			category uint64
			der      []byte
		}
		entries := make([]certEntry, 0, len(certObjs))
		for _, obj := range certObjs {
			attrs, gerr := client.getAttributeValue(sess, obj, []uint32{ckaValue, ckaID, ckaCertificateCategory})
			if gerr != nil {
				continue
			}
			var e certEntry
			for _, a := range attrs {
				if !a.Valid {
					continue
				}
				switch a.Type {
				case ckaValue:
					e.der = a.Value
				case ckaID:
					if len(a.Value) > 0 {
						e.id = a.Value[0]
					}
				case ckaCertificateCategory:
					e.category = a.ULong
				}
			}
			if len(e.der) > 0 {
				entries = append(entries, e)
			}
		}

		if len(entries) > 0 {
			var leaf [][]byte
			var chain [][]byte
			for _, e := range entries {
				if e.category == ckCertCategoryTokenUser || e.id == 1 {
					leaf = append(leaf, e.der)
				} else {
					chain = append(chain, e.der)
				}
			}
			out := append(leaf, chain...)
			if len(out) > 0 {
				return out, nil
			}
		}
	}

	if certPath != "" {
		return loadPEMCertificates(certPath)
	}
	return nil, errors.New("no certificate objects in pkcs11 token and no fallback cert file")
}

func loadPEMCertificates(path string) ([][]byte, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ders [][]byte
	for len(pemBytes) > 0 {
		var block *pem.Block
		block, pemBytes = pem.Decode(pemBytes)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			ders = append(ders, block.Bytes)
		}
	}
	return ders, nil
}
