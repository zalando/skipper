package certregistry

import (
	"crypto/tls"
	"reflect"
	"testing"
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
		_, found := cr.getCertByKey("foo")
		if !found {
			t.Error("failed to read certificate")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {
		newcert := getFakeHostTLSCert("bar.org")
		newhosts := make([]string, 1)
		newhosts[0] = "bar.org"

		cr := NewCertRegistry()
		cr.SyncCert("foo", hosts, cert)
		cert1, _ := cr.getCertByKey("foo")
		cr.SyncCert("foo", newhosts, newcert)
		cert2, _ := cr.getCertByKey("foo")
		if equalCert(cert1, cert2) {
			t.Error("foo key was not updated")
		}

	})

	t.Run("sync existing equal certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("bar", hosts, cert)
		changed := cr.SyncCert("bar", hosts, cert)
		if changed {
			t.Error("equal certificate was updated")
		}
	})

	t.Run("get non existent cert", func(t *testing.T) {
		cr := NewCertRegistry()
		_, found := cr.getCertByKey("foobar")
		if found {
			t.Error("non existent certificate was found")
		}
	})

	t.Run("get cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert("foo", hosts, cert)
		crt, err := cr.GetCertFromHello(hello)
		if err != nil {
			t.Error("failed to read certificate from hello")
		}
		if !reflect.DeepEqual(crt.Certificate, cert.Certificate) {
			t.Error("failed to read certificate from hello")
		}
	})

	t.Run("get default cert from hello", func(t *testing.T) {
		cr := NewCertRegistry()
		_, err := cr.GetCertFromHello(hello)
		if err != nil {
			t.Error("failed to read default certificate from hello")
		}
	})
}
