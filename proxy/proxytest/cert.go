package proxytest

import (
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/zalando/skipper/net/nettest"
)

func NewCertificate(hosts ...string) tls.Certificate {
	now := time.Now()
	dnsNames, ipAddrs := nettest.SplitHosts(hosts)
	certPEM, keyPEM, _, _, err := nettest.GenCert(nettest.CertOptions{
		CN:           "Zalando SE",
		SanDNS:       dnsNames,
		SanIP:        ipAddrs,
		NotBefore:    now,
		NotAfter:     now.Add(1 * time.Hour),
		ExtKeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		panic(err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return cert
}
