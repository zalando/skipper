package certregistry

import (
	"crypto/tls"
	"testing"
)

func TestCertRegistry(t *testing.T) {

	cert := tls.Certificate{}

	newcert := tls.Certificate{}

    t.Run("sync new certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("example.org", &cert)
		_, err := cr.getCertByKey("example.org")
		if err != nil {
			t.Error("failed to read certificate")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("example.org", &cert)
		cr.SyncCert("example.org", &newcert)
	})
}
