package certregistry

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertRegistry(t *testing.T) {

	cert := getFakeHostTLSCert("foo.org")
	hosts := make([]string, 1)
	hosts[0] = "foo.org"

	hello := &tls.ClientHelloInfo{
		ServerName: "foo.org",
	}

    t.Run("sync new certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("foo", hosts, cert)
		_, err := cr.getCertByKey("foo")
		if err != nil {
			t.Error("failed to read certificate")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {
		newcert := getFakeHostTLSCert("bar.org")
		newhosts := make([]string, 1)
		newhosts[0] = "foo.org"

		cr := NewCertRegistry()
		cr.SyncCert("foo", hosts, cert)
		cr.SyncCert("foo", newhosts, newcert)
	})

	t.Run("get non existent cert", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.getCertByKey("foobar")
        require.Error(t, err)
	})

	t.Run("get cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.GetCertFromHello(hello)
		if err != nil {
			t.Error("failed to read certificate from hello")
		}
	})

	t.Run("get default cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.GetCertFromHello(hello)
		if err != nil {
			t.Error("failed to read certificate from hello")
		}
	})
}
