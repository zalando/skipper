package certregistry

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	tenYears = time.Hour * 24 * 365 * 10
)

type caInfra struct {
	sync.Once
	err       error
	chainKey  *rsa.PrivateKey
	chainCert *x509.Certificate
}

func certValidMatchFunction(err error, expect, c *tls.Certificate) bool {
	return err != nil || c != expect
}

func certInvalidMatchFunction(err error, expect, c *tls.Certificate) bool {
	return err == nil && c == expect
}

type certCondition func(error, *tls.Certificate, *tls.Certificate) bool

var ca = caInfra{}

func createDummyCertDetail(t *testing.T, arn string, altNames []string, notBefore, notAfter time.Time) *tls.Certificate {
	ca.Do(func() {
		caKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate CA key: %v", err)
			return
		}

		caCert := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"Testing CA"},
			},
			NotBefore: time.Time{},
			NotAfter:  time.Now().Add(tenYears),

			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,

			IsCA: true,
		}
		caBody, err := x509.CreateCertificate(rand.Reader, &caCert, &caCert, caKey.Public(), caKey)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate CA certificate: %v", err)
			return
		}
		caReparsed, err := x509.ParseCertificate(caBody)
		if err != nil {
			ca.err = fmt.Errorf("unable to parse CA certificate: %v", err)
			return
		}
		roots := x509.NewCertPool()
		roots.AddCert(caReparsed)

		chainKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate sub-CA key: %v", err)
			return
		}
		chainCert := x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				Organization: []string{"Testing Sub-CA"},
			},
			NotBefore: time.Time{},
			NotAfter:  time.Now().Add(tenYears),

			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,

			IsCA: true,
		}
		chainBody, err := x509.CreateCertificate(rand.Reader, &chainCert, caReparsed, chainKey.Public(), caKey)
		if err != nil {
			ca.err = fmt.Errorf("unable to generate sub-CA certificate: %v", err)
			return
		}
		chainReparsed, err := x509.ParseCertificate(chainBody)
		if err != nil {
			ca.err = fmt.Errorf("unable to parse sub-CA certificate: %v", err)
			return
		}

		ca.chainKey = chainKey
		ca.chainCert = chainReparsed
	})

	certKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		require.NoErrorf(t, err, "unable to generate certificate key")
	}
	cert := x509.Certificate{
		SerialNumber: big.NewInt(3),
		DNSNames:     altNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	body, err := x509.CreateCertificate(rand.Reader, &cert, ca.chainCert, certKey.Public(), ca.chainKey)
	if err != nil {
		require.NoErrorf(t, err, "unable to generate certificate")
	}

	crt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: body})

	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(certKey)})

	certificate, err := tls.X509KeyPair([]byte(crt), []byte(key))
	if err != nil {
		log.Errorf("failed to generate fake serial number: %v", err)
	}

	return &certificate
}

func TestCertRegistry(t *testing.T) {

	cert := getFakeHostTLSCert("foo.org")
	host := "foo.org"

	hello := &tls.ClientHelloInfo{
		ServerName: "foo.org",
	}

	t.Run("sync new certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert(host, cert)
		_, found := cr.getCertByKey("foo.org")
		if !found {
			t.Error("failed to read certificate")
		}
	})

	t.Run("sync a nil certificate", func(t *testing.T) {
		cr := NewCertRegistry()
		cr.SyncCert(host, nil)
		_, found := cr.getCertByKey("foo.org")
		if found {
			t.Error("nil certificate should not sync")
		}
	})

	t.Run("sync existing certificate", func(t *testing.T) {
		newcert := getFakeHostTLSCert("bar.org")

		cr := NewCertRegistry()
		cr.SyncCert(host, cert)
		cert1, _ := cr.getCertByKey("foo.org")
		cr.SyncCert(host, newcert)
		cert2, _ := cr.getCertByKey("foo.org")
		if equalCert(cert1, cert2) {
			t.Error("host cert was not updated")
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
		cr.SyncCert(host, cert)
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

	t.Run("get cert from hello - multiple matches", func(t *testing.T) {
		newcert := getFakeHostTLSCert("foo.org")

		cr := NewCertRegistry()
		cr.SyncCert(host, cert)
		cr.SyncCert(host, newcert)
		reply, err := cr.GetCertFromHello(hello)
		if err != nil {
			t.Error("failed to certificate from hello")
		}
		if !reflect.DeepEqual(reply, newcert) {
			t.Error("failed to read best certificate from hello")
		}
	})
}

func TestFindBestMatchingCertificate(t *testing.T) {
	domain := "example.org"
	wildcardDomain := "*." + domain
	invalidDomain := "invalid.org"
	invalidWildcardDomain := "*." + invalidDomain
	validHostname := "foo." + domain
	invalidHostname := "foo." + invalidDomain

	now := time.Now().Truncate(time.Millisecond)
	currentTime = func() time.Time { return now }

	before := now.Add(-time.Hour * 24 * 7)
	after := now.Add(time.Hour*24*7 + 1*time.Second)
	dummyArn := "DUMMY"

	// simple cert
	validCert := createDummyCertDetail(t, dummyArn, []string{validHostname}, before, after)
	validWildcardCert := createDummyCertDetail(t, dummyArn, []string{wildcardDomain}, before, after)
	invalidDomainCert := createDummyCertDetail(t, dummyArn, []string{invalidDomain}, before, after)
	invalidWildcardCert := createDummyCertDetail(t, dummyArn, []string{invalidWildcardDomain}, before, after)

	// AlternateName certs
	saValidCert := createDummyCertDetail(t, dummyArn, []string{validHostname, invalidDomain, invalidHostname, invalidWildcardDomain}, before, after)
	saValidWildcardCert := createDummyCertDetail(t, dummyArn, []string{invalidDomain, invalidHostname, invalidWildcardDomain, wildcardDomain}, before, after)
	saMultipleValidCert := createDummyCertDetail(t, dummyArn, []string{wildcardDomain, validHostname, invalidDomain, invalidHostname, invalidWildcardDomain}, before, after)

	// simple invalid time cases
	invalidTimeCert1 := createDummyCertDetail(t, dummyArn, []string{domain}, after, before)
	invalidTimeCert2 := createDummyCertDetail(t, dummyArn, []string{domain}, after, after)
	invalidTimeCert3 := createDummyCertDetail(t, dummyArn, []string{domain}, before, before)

	// tricky times with multiple valid certs
	validCertForOneDay := createDummyCertDetail(t, dummyArn, []string{validHostname}, before, now.Add(time.Hour*24))
	validCertForSixDays := createDummyCertDetail(t, dummyArn, []string{validHostname}, before, now.Add(time.Hour*24*6))
	validCertForTenDays := createDummyCertDetail(t, dummyArn, []string{validHostname}, before, now.Add(time.Hour*24*6))
	validCertForOneYear := createDummyCertDetail(t, dummyArn, []string{validHostname}, before, now.Add(time.Hour*24*365))
	validCertSinceOneDay := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24), after)
	validCertSinceSixDays := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*6), after)
	validCertSinceOneYear := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*365), after)
	validCertForOneYearSinceOneDay := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24), now.Add(time.Hour*24*365))
	validCertForOneYearSinceSixDays := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*6), now.Add(time.Hour*24*365))
	validCertForOneYearSinceOneYear := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*365), now.Add(time.Hour*24*365))

	validCertFor6dUntill1y := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*6), now.Add(time.Hour*24*365))
	validCertFor6dUntill6d := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*6), now.Add(time.Hour*24*6))
	validCertFor6dUntill10d := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*6), now.Add(time.Hour*24*10))
	validCertFor1dUntill6d := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*1), now.Add(time.Hour*24*6))
	validCertFor1dUntill7d1sLess := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*1), now.Add(time.Hour*24*7-time.Second*1))
	validCertFor1dUntill7d1s := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*1), now.Add(time.Hour*24*7+time.Second*1))
	validCertFor1dUntill10d := createDummyCertDetail(t, dummyArn, []string{validHostname}, now.Add(-time.Hour*24*1), now.Add(time.Hour*24*10))

	for _, ti := range []struct {
		msg       string
		hostname  string
		cert      []*tls.Certificate
		expect    *tls.Certificate
		condition certCondition
	}{
		{
			msg:       "Not found best match",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCert},
			expect:    validCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found wildcard as best match",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validWildcardCert},
			expect:    validWildcardCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match of multiple valid certs",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCert, validWildcardCert},
			expect:    validCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match of multiple certs one wildcard valid",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidDomainCert, validWildcardCert, invalidWildcardCert},
			expect:    validWildcardCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match of multiple certs one valid",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidDomainCert, validCert, invalidWildcardCert},
			expect:    validCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Found best match for invalid hostname",
			hostname:  invalidHostname,
			cert:      []*tls.Certificate{validCert},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Found best match for invalid cert",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidDomainCert},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Found best match for invalid wildcardcert",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidWildcardCert},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Found best match for multiple invalid certs",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidWildcardCert, invalidDomainCert},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Not found best match of AlternateName cert with one valid and multiple invalid names",
			hostname:  validHostname,
			cert:      []*tls.Certificate{saValidCert},
			expect:    saValidCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match of AlternateName cert with one valid wildcard and multiple invalid names",
			hostname:  validHostname,
			cert:      []*tls.Certificate{saValidWildcardCert},
			expect:    saValidWildcardCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match of AlternateName cert with multiple valid and multiple invalid names",
			hostname:  validHostname,
			cert:      []*tls.Certificate{saMultipleValidCert},
			expect:    saMultipleValidCert,
			condition: certValidMatchFunction,
		}, {
			msg:       "Found best match for invalid time cert 1",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidTimeCert1},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Found best match for invalid time cert 2",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidTimeCert2},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Found best match for invalid time cert 3",
			hostname:  validHostname,
			cert:      []*tls.Certificate{invalidTimeCert3},
			expect:    nil,
			condition: certInvalidMatchFunction,
		}, {
			msg:       "Not found best match tricky cert NotAfter 1 day compared to 6 days",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForOneDay, validCertForSixDays},
			expect:    validCertForSixDays,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match tricky cert NotAfter 365 days compared to 1 day",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForOneYear, validCertForOneDay},
			expect:    validCertForOneYear,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match tricky cert NotAfter 365 days compared to 6 day",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForOneYear, validCertForSixDays},
			expect:    validCertForOneYear,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match tricky cert NotAfter 365 days compared to 10 day",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForTenDays, validCertForOneYear},
			expect:    validCertForOneYear,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore 6 days compared to 1 day",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertSinceOneDay, validCertSinceSixDays}, // FIXME: this is by order
			expect:    validCertSinceOneDay,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore 6 days compared to 365 days",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertSinceSixDays, validCertSinceOneYear},
			expect:    validCertSinceSixDays,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore 6 days compared to 365 days another order by cert",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertSinceOneYear, validCertSinceSixDays},
			expect:    validCertSinceSixDays,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore 1 days compared to 365 days and both valid for 1 year",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForOneYearSinceOneDay, validCertForOneYearSinceOneYear},
			expect:    validCertForOneYearSinceOneDay,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore 6 days compared to 365 days and both valid for 1 year",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertForOneYearSinceOneYear, validCertForOneYearSinceSixDays},
			expect:    validCertForOneYearSinceSixDays,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newest first) tricky cert NotBefore/NotAfter 6d/1y compared to 1d/10d",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertFor6dUntill1y, validCertFor1dUntill10d},
			expect:    validCertFor1dUntill10d,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (newer first) tricky cert NotBefore/NotAfter 1d/7d1s compared to 6d/10d",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertFor1dUntill7d1s, validCertFor6dUntill10d},
			expect:    validCertFor1dUntill7d1s,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (longer first) tricky cert NotBefore/NotAfter 6d/6d compared to 6d/1y",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertFor6dUntill6d, validCertForOneYearSinceSixDays},
			expect:    validCertForOneYearSinceSixDays,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (longer first) tricky cert NotBefore/NotAfter 1d/6d compared to 6d/10d",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertFor1dUntill6d, validCertFor6dUntill10d},
			expect:    validCertFor6dUntill10d,
			condition: certValidMatchFunction,
		}, {
			msg:       "Not found best match (longer first) tricky cert NotBefore/NotAfter 6d/10d compared to 1d/7d-1s",
			hostname:  validHostname,
			cert:      []*tls.Certificate{validCertFor6dUntill10d, validCertFor1dUntill7d1sLess},
			expect:    validCertFor6dUntill10d,
			condition: certValidMatchFunction,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			if c, err := getBestMatchingCertificate(ti.hostname, ti.cert); ti.condition(err, c, ti.expect) {
				t.Errorf("%s: for host: %s expected %v, got %v, err: %v", ti.msg, ti.hostname, ti.expect, c, err)
			}

		})
	}

}

func TestGlob(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		pattern string
		subj    string
		expect  bool
	}{
		{
			msg:     "Not found exact match",
			pattern: "www.foo.org",
			subj:    "www.foo.org",
			expect:  true,
		}, {
			msg:     "Not found simple glob",
			pattern: "*",
			subj:    "www.foo.org",
			expect:  true,
		}, {
			msg:     "Not found simple match",
			pattern: "*.foo.org",
			subj:    "www.foo.org",
			expect:  true,
		}, {
			msg:     "Found wrong simple match prefix",
			pattern: "www.foo.org",
			subj:    "wwww.foo.org",
			expect:  false,
		}, {
			msg:     "Found wrong simple match suffix",
			pattern: "www.foo.org",
			subj:    "www.foo.orgg",
			expect:  false,
		}, {
			msg:     "Found wrong complex match",
			pattern: "*.foo.org",
			subj:    "www.baz.foo.org",
			expect:  false,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			if prefixGlob(ti.pattern, ti.subj) != ti.expect {
				t.Errorf("%s: for pattern: %s and subj: %s, expected %v", ti.msg, ti.pattern, ti.subj, ti.expect)
			}

		})
	}

}
