package certregistry

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Config holds a certificate registry TLS configuration.
type Config struct {
	ClientAuth  tls.ClientAuthType
	Certificate tls.Certificate
}

// CertRegistry object holds TLS Config to be used to terminate TLS connections
// ensuring synchronized access to them.
type CertRegistry struct {
	mu     sync.Mutex
	lookup map[string]*tlsConfigWrapper

	// defaultTLSConfig is TLS config to be used as a base config for all host configs.
	defaultConfig *tls.Config
}

// tlsConfigWrapper holds the tls.Config and a hash of a host configuration.
type tlsConfigWrapper struct {
	config *tls.Config
	hash   []byte
}

// NewCertRegistry initializes the certificate registry.
func NewCertRegistry() *CertRegistry {
	l := make(map[string]*tlsConfigWrapper)

	return &CertRegistry{
		lookup:        l,
		defaultConfig: &tls.Config{},
	}
}

// Configures TLS for the host if no configuration exists or
// if config certificate is valid (`NotBefore` field) after previously configured certificate.
func (r *CertRegistry) SetTLSConfig(host string, config *Config) error {
	if config == nil {
		return fmt.Errorf("cannot configure nil tls config")
	}
	// loading parsed leaf certificate to certificate
	leaf, err := x509.ParseCertificate(config.Certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed parsing leaf certificate: %w", err)
	}
	config.Certificate.Leaf = leaf

	// Get tls.config and hash from the config
	tlsConfig, configHash := r.configToTLSConfig(config)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if the config is already set
	curr, found := r.lookup[host]
	if found && bytes.Equal(curr.hash, configHash) {
		return nil
	}

	if found && !bytes.Equal(curr.hash, configHash) {
		if !config.Certificate.Leaf.NotBefore.After(curr.config.Certificates[0].Leaf.NotBefore) {
			return nil
		}
	}

	log.Infof("setting tls config in registry - %s", host)
	wrapper := &tlsConfigWrapper{
		config: tlsConfig,
		hash:   configHash,
	}
	r.lookup[host] = wrapper

	return nil
}

// GetConfigFromHello reads the SNI from a TLS client and returns the appropriate config.
func (r *CertRegistry) GetConfigFromHello(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	r.mu.Lock()
	entry, found := r.lookup[hello.ServerName]
	r.mu.Unlock()
	if found {
		return entry.config, nil
	}
	return entry.config, nil
}

// configToTLSConfig converts a Config to a tls.Config and returns the hash of the config.
func (r *CertRegistry) configToTLSConfig(config *Config) (*tls.Config, []byte) {
	if config == nil {
		return nil, nil
	}

	var hash []byte

	tlsConfig := r.defaultConfig.Clone()

	// Add client auth settings
	tlsConfig.ClientAuth = config.ClientAuth
	hash = append(hash, byte(config.ClientAuth>>8), byte(config.ClientAuth))

	// Add certificate
	tlsConfig.Certificates = append(tlsConfig.Certificates, config.Certificate)
	for _, certData := range config.Certificate.Certificate {
		hash = append(hash, certData...)
	}

	return tlsConfig, hash
}

// SetDefaultTLSConfig sets the default TLS config which should be used as a base for all host specific configs.
func (r *CertRegistry) SetDefaultTLSConfig(config *tls.Config) {
	r.defaultConfig = config
}
