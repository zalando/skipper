//go:build awsnitro

// Package nitro provides TLS configuration for AWS Nitro Enclaves using
// ACM (Certificate Manager) for Nitro Enclaves. Private keys remain sealed
// inside the enclave; only RSA signing operations cross the vsock boundary.
package nitro

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	pkcs11key "github.com/letsencrypt/pkcs11key/v4"
)

// Config holds the parameters needed to connect to the PKCS#11 token
// exposed by the nitro-enclaves-acm service.
type Config struct {
	// Module is the filesystem path to the PKCS#11 shared library,
	// typically /usr/lib64/pkcs11/p11-kit-client.so on the parent EC2 instance.
	Module string
	// TokenLabel is the PKCS#11 token label assigned by nitro-enclaves-acm.
	TokenLabel string
	// PIN is the PKCS#11 user PIN. ACM tokens typically do not require a PIN.
	PIN string
	// CertFile is the path to the PEM certificate chain file written by
	// nitro-enclaves-acm, typically /etc/nitro_enclaves/acm.cert.pem.
	CertFile string
}

// TLSConfig returns a *tls.Config that uses the ACM certificate whose private
// key lives inside the Nitro Enclave. Returns nil, nil when cfg.Module is
// empty (Nitro not configured).
func TLSConfig(cfg Config) (*tls.Config, error) {
	if cfg.Module == "" {
		return nil, nil
	}

	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("read cert file %q: %w", cfg.CertFile, err)
	}

	var derChain [][]byte
	var leaf *x509.Certificate

	rest := certPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		if leaf == nil {
			leaf = cert
		}
		derChain = append(derChain, block.Bytes)
	}
	if leaf == nil {
		return nil, fmt.Errorf("no certificate found in %q", cfg.CertFile)
	}

	signer, err := pkcs11key.New(cfg.Module, cfg.TokenLabel, cfg.PIN, leaf.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("pkcs11key init (module=%q token=%q): %w", cfg.Module, cfg.TokenLabel, err)
	}

	tlsCert := tls.Certificate{
		Certificate: derChain,
		PrivateKey:  signer,
		Leaf:        leaf,
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}
