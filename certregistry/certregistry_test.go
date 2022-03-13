package certregistry

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertRegistry(t *testing.T) {

	cert := getFakeHostTLSCert("foo.org")

	newcert := getFakeHostTLSCert("bar.org")

	hello := tls.ClientHelloInfo{
		ServerName: "example.org",
	}

    t.Run("sync new certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("example.org", cert)
		_, err := cr.getCertByKey("example.org")
		if err != nil {
			t.Error("failed to read certificate")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("example.org", cert)
		cr.SyncCert("example.org", newcert)
	})

	t.Run("get default certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.getCertByKey(defaultHost)
		if err != nil {
			t.Error("failed to read certificate")
		}
	})

	t.Run("get non existent cert", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.getCertByKey("foobar.org")
        require.Error(t, err)
	})

	t.Run("get default cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.GetCertFromHello(&hello)
		if err != nil {
			t.Error("failed to read certificate from hello")
		}
	})
}
