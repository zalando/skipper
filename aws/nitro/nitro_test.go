//go:build awsnitro

package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/secrets/certregistry"
)

type testCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pool *x509.CertPool
}

var (
	caOnce sync.Once
	ca     testCA
)

func getCA(t testing.TB) testCA {
	caOnce.Do(func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "test ca"},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			KeyUsage:              x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
		require.NoError(t, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(t, err)
		pool := x509.NewCertPool()
		pool.AddCert(cert)
		ca = testCA{cert: cert, key: key, pool: pool}
	})
	return ca
}

func newKey(t testing.TB, typ string) crypto.Signer {
	t.Helper()
	var (
		k   crypto.Signer
		err error
	)
	switch typ {
	case "rsa":
		k, err = rsa.GenerateKey(rand.Reader, 2048)
	case "p256":
		k, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "p384":
		k, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	default:
		t.Fatalf("unknown key type %s", typ)
	}
	require.NoError(t, err)
	return k
}

func writeChain(t testing.TB, path string, key crypto.Signer, notBefore time.Time, hosts ...string) {
	t.Helper()
	c := getCA(t)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hosts[0]},
		DNSNames:     hosts,
		NotBefore:    notBefore,
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, key.Public(), c.key)
	require.NoError(t, err)

	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.cert.Raw})...)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func handshake(t *testing.T, cr *certregistry.CertRegistry, serverName string, version uint16) error {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	server := tls.Server(serverConn, &tls.Config{
		GetCertificate: RequireSNI(cr.GetCertFromHello, nil),
		MinVersion:     tls.VersionTLS12,
	})
	client := tls.Client(clientConn, &tls.Config{
		RootCAs:            getCA(t).pool,
		ServerName:         serverName,
		InsecureSkipVerify: serverName == "",
		MinVersion:         version,
		MaxVersion:         version,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		err := server.HandshakeContext(ctx)
		if err != nil {
			serverConn.Close()
		}
		errc <- err
	}()
	clientErr := client.HandshakeContext(ctx)
	if clientErr != nil {
		clientConn.Close()
	}
	if serverErr := <-errc; serverErr != nil {
		return serverErr
	}
	return clientErr
}

func testOptions(f *fakeServer, certs ...Certificate) Options {
	return Options{
		Certificates:     certs,
		SessionsPerToken: 2,
		Timeout:          time.Second,
		RefreshInterval:  time.Hour,
		Metrics:          &metricstest.MockMetrics{},
		dial:             f.dial,
	}
}

func TestProviderHandshake(t *testing.T) {
	for _, keyType := range []string{"rsa", "p256", "p384"} {
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			t.Run(keyType+"/"+tls.VersionName(version), func(t *testing.T) {
				key := newKey(t, keyType)
				chain := filepath.Join(t.TempDir(), "chain.pem")
				writeChain(t, chain, key, time.Now().Add(-time.Minute), "a.example.org", "*.example.com")

				f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
				defer f.close()

				cr := certregistry.NewCertRegistry()
				p, err := NewProvider(testOptions(f, Certificate{TokenLabel: "acm", ChainPath: chain}), cr)
				require.NoError(t, err)
				defer p.Close()

				assert.NoError(t, handshake(t, cr, "a.example.org", version))
				assert.NoError(t, handshake(t, cr, "b.example.com", version))
				assert.ErrorContains(t, handshake(t, cr, "", version), ErrNoSNI.Error())
				assert.ErrorContains(t, handshake(t, cr, "unknown.example.org", version), ErrUnknownServerName.Error())
			})
		}
	}
}

func TestSignerMechanisms(t *testing.T) {
	rsaKey := newKey(t, "rsa").(*rsa.PrivateKey)
	ecKey := newKey(t, "p256").(*ecdsa.PrivateKey)
	digest := sha256.Sum256([]byte("data"))

	for _, tt := range []struct {
		name   string
		key    crypto.Signer
		opts   crypto.SignerOpts
		verify func([]byte) error
	}{
		{
			name: "rsa pkcs1v15",
			key:  rsaKey,
			opts: crypto.SHA256,
			verify: func(sig []byte) error {
				return rsa.VerifyPKCS1v15(&rsaKey.PublicKey, crypto.SHA256, digest[:], sig)
			},
		},
		{
			name: "rsa pss salt equals hash",
			key:  rsaKey,
			opts: &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256},
			verify: func(sig []byte) error {
				return rsa.VerifyPSS(&rsaKey.PublicKey, crypto.SHA256, digest[:], sig, &rsa.PSSOptions{SaltLength: sha256.Size})
			},
		},
		{
			name: "rsa pss salt auto",
			key:  rsaKey,
			opts: &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto, Hash: crypto.SHA256},
			verify: func(sig []byte) error {
				return rsa.VerifyPSS(&rsaKey.PublicKey, crypto.SHA256, digest[:], sig, nil)
			},
		},
		{
			name: "ecdsa",
			key:  ecKey,
			opts: crypto.SHA256,
			verify: func(sig []byte) error {
				if !ecdsa.VerifyASN1(&ecKey.PublicKey, digest[:], sig) {
					return assert.AnError
				}
				return nil
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: tt.key}}})
			defer f.close()

			s, err := newSigner(testOptions(f), Certificate{TokenLabel: "acm"}, nil, tt.key.Public())
			require.NoError(t, err)
			defer s.Close()

			sig, err := s.Sign(rand.Reader, digest[:], tt.opts)
			require.NoError(t, err)
			assert.NoError(t, tt.verify(sig))
		})
	}
}

func TestSignerErrors(t *testing.T) {
	key := newKey(t, "rsa")
	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	s, err := newSigner(testOptions(f), Certificate{TokenLabel: "acm"}, nil, key.Public())
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Sign(nil, []byte("short"), crypto.SHA256)
	assert.ErrorContains(t, err, "digest length")

	_, err = s.Sign(nil, make([]byte, 16), crypto.MD5)
	assert.ErrorContains(t, err, "unsupported hash")

	_, err = newSigner(testOptions(f), Certificate{TokenLabel: "acm"}, nil, []byte("not a key"))
	assert.ErrorContains(t, err, "unsupported public key type")
}

func TestSignerReconnect(t *testing.T) {
	key := newKey(t, "p256")
	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	o := testOptions(f)
	s, err := newSigner(o, Certificate{TokenLabel: "acm"}, nil, key.Public())
	require.NoError(t, err)
	defer s.Close()

	digest := sha256.Sum256([]byte("data"))
	_, err = s.Sign(nil, digest[:], crypto.SHA256)
	require.NoError(t, err)

	f.disconnect()

	_, err = s.Sign(nil, digest[:], crypto.SHA256)
	require.NoError(t, err)
	assert.Equal(t, 2, f.count(callInitialize))
}

func TestSignerConcurrent(t *testing.T) {
	key := newKey(t, "p256")
	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	s, err := newSigner(testOptions(f), Certificate{TokenLabel: "acm"}, nil, key.Public())
	require.NoError(t, err)
	defer s.Close()

	digest := sha256.Sum256([]byte("data"))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := s.Sign(nil, digest[:], crypto.SHA256)
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	assert.LessOrEqual(t, f.count(callInitialize), 2)
}

func TestNewProviderErrors(t *testing.T) {
	key := newKey(t, "p256")
	otherKey := newKey(t, "p256")
	dir := t.TempDir()
	chain := filepath.Join(dir, "chain.pem")
	writeChain(t, chain, key, time.Now().Add(-time.Minute), "a.example.org")
	pinFile := filepath.Join(dir, "pin")
	require.NoError(t, os.WriteFile(pinFile, []byte("1234\n"), 0o600))
	wrongPinFile := filepath.Join(dir, "wrong-pin")
	require.NoError(t, os.WriteFile(wrongPinFile, []byte("0000"), 0o600))
	invalidChain := filepath.Join(dir, "invalid.pem")
	require.NoError(t, os.WriteFile(invalidChain, []byte("invalid"), 0o600))

	for _, tt := range []struct {
		name     string
		token    *fakeToken
		cert     Certificate
		pinFile  string
		registry *certregistry.CertRegistry
		wantErr  string
	}{
		{
			name:    "success with pin",
			token:   &fakeToken{label: "acm", pin: "1234", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			pinFile: pinFile,
		},
		{
			name: "success with key label",
			token: &fakeToken{label: "acm", keys: []fakeKey{
				{label: "other", signer: otherKey},
				{label: "mine", signer: key},
			}},
			cert: Certificate{TokenLabel: "acm", ChainPath: chain, KeyLabel: "mine"},
		},
		{
			name:    "token not found",
			token:   &fakeToken{label: "other", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			wantErr: `token "acm" not found`,
		},
		{
			name:    "key does not match certificate",
			token:   &fakeToken{label: "acm", keys: []fakeKey{{signer: otherKey}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			wantErr: "does not match certificate",
		},
		{
			name:    "login required without pin",
			token:   &fakeToken{label: "acm", pin: "1234", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			wantErr: "requires login",
		},
		{
			name:    "wrong pin",
			token:   &fakeToken{label: "acm", pin: "1234", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			pinFile: wrongPinFile,
			wantErr: "CKR_PIN_INCORRECT",
		},
		{
			name:    "missing pin file",
			token:   &fakeToken{label: "acm", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			pinFile: filepath.Join(dir, "missing"),
			wantErr: "pin file",
		},
		{
			name: "multiple keys without key label",
			token: &fakeToken{label: "acm", keys: []fakeKey{
				{label: "other", signer: otherKey},
				{label: "mine", signer: key},
			}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: chain},
			wantErr: "found 2 private keys",
		},
		{
			name:    "missing chain",
			token:   &fakeToken{label: "acm", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: filepath.Join(dir, "missing.pem")},
			wantErr: "failed to read certificate chain",
		},
		{
			name:    "invalid chain",
			token:   &fakeToken{label: "acm", keys: []fakeKey{{signer: key}}},
			cert:    Certificate{TokenLabel: "acm", ChainPath: invalidChain},
			wantErr: "no certificate found",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(tt.token)
			defer f.close()

			o := testOptions(f, tt.cert)
			o.PINFile = tt.pinFile
			p, err := NewProvider(o, certregistry.NewCertRegistry())
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			p.Close()
		})
	}

	t.Run("invalid options", func(t *testing.T) {
		_, err := NewProvider(Options{Certificates: []Certificate{{}}}, nil)
		assert.ErrorContains(t, err, "certificate registry")
		_, err = NewProvider(Options{}, certregistry.NewCertRegistry())
		assert.ErrorContains(t, err, "at least one certificate")
		_, err = NewProvider(Options{Certificates: []Certificate{{}}}, certregistry.NewCertRegistry())
		assert.ErrorContains(t, err, "cid and port")
	})

	t.Run("unsupported protocol version", func(t *testing.T) {
		f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
		f.version = 1
		defer f.close()

		_, err := NewProvider(testOptions(f, Certificate{TokenLabel: "acm", ChainPath: chain}), certregistry.NewCertRegistry())
		assert.ErrorContains(t, err, "unsupported protocol version")
	})
}

func TestProviderRotation(t *testing.T) {
	const host = "a.example.org"
	key := newKey(t, "p256")
	chain := filepath.Join(t.TempDir(), "chain.pem")
	writeChain(t, chain, key, time.Now().Add(-time.Hour), host)

	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	cr := certregistry.NewCertRegistry()
	p, err := NewProvider(testOptions(f, Certificate{TokenLabel: "acm", ChainPath: chain}), cr)
	require.NoError(t, err)
	defer p.Close()

	get := func() *tls.Certificate {
		c, err := cr.GetCertFromHello(&tls.ClientHelloInfo{ServerName: host})
		require.NoError(t, err)
		require.NotNil(t, c)
		return c
	}
	first := get()

	t.Run("unchanged chain", func(t *testing.T) {
		require.NoError(t, p.load(p.tokens[0]))
		assert.Same(t, first, get())
	})

	t.Run("renewed certificate with same key reuses signer", func(t *testing.T) {
		writeChain(t, chain, key, time.Now().Add(-30*time.Minute), host)
		require.NoError(t, p.load(p.tokens[0]))
		renewed := get()
		assert.NotSame(t, first, renewed)
		assert.Same(t, first.PrivateKey, renewed.PrivateKey)
	})

	t.Run("renewed certificate with new key", func(t *testing.T) {
		before := get()
		rotatedKey := newKey(t, "p256")
		f.setToken(0, &fakeToken{label: "acm", keys: []fakeKey{{signer: rotatedKey}}})
		writeChain(t, chain, rotatedKey, time.Now().Add(-time.Minute), host)

		require.NoError(t, p.load(p.tokens[0]))
		renewed := get()
		assert.NotSame(t, before.PrivateKey, renewed.PrivateKey)
		assert.NoError(t, handshake(t, cr, host, tls.VersionTLS13))

		p.mu.Lock()
		assert.Len(t, p.retired, 1)
		p.mu.Unlock()
	})

	t.Run("invalid chain keeps certificate", func(t *testing.T) {
		before := get()
		require.NoError(t, os.WriteFile(chain, []byte("invalid"), 0o600))
		assert.Error(t, p.load(p.tokens[0]))
		assert.Same(t, before, get())
	})
}

func TestNitroPriority(t *testing.T) {
	const host = "a.example.org"
	key := newKey(t, "p256")
	dir := t.TempDir()
	chain := filepath.Join(dir, "chain.pem")
	writeChain(t, chain, key, time.Now().Add(-time.Hour), host)

	otherKey := newKey(t, "p256")
	otherChain := filepath.Join(dir, "other.pem")
	writeChain(t, otherChain, otherKey, time.Now().Add(-time.Minute), host)
	other, err := parseChain(mustRead(t, otherChain))
	require.NoError(t, err)
	other.PrivateKey = otherKey

	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	cr := certregistry.NewCertRegistry()
	p, err := NewProvider(testOptions(f, Certificate{TokenLabel: "acm", ChainPath: chain}), cr)
	require.NoError(t, err)
	defer p.Close()

	require.NoError(t, cr.ConfigureCertificate(host, other))

	c, err := cr.GetCertFromHello(&tls.ClientHelloInfo{ServerName: host})
	require.NoError(t, err)
	assert.IsType(t, &signer{}, c.PrivateKey)
}

func TestProviderClose(t *testing.T) {
	key := newKey(t, "p256")
	chain := filepath.Join(t.TempDir(), "chain.pem")
	writeChain(t, chain, key, time.Now().Add(-time.Minute), "a.example.org")

	f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
	defer f.close()

	cr := certregistry.NewCertRegistry()
	p, err := NewProvider(testOptions(f, Certificate{TokenLabel: "acm", ChainPath: chain}), cr)
	require.NoError(t, err)
	require.NoError(t, p.Close())
	require.NoError(t, p.Close())

	assert.Equal(t, 1, f.count(callCloseSession))
	assert.Equal(t, 1, f.count(callFinalize))

	c, err := cr.GetCertFromHello(&tls.ClientHelloInfo{ServerName: "a.example.org"})
	require.NoError(t, err)
	_, err = c.PrivateKey.(crypto.Signer).Sign(nil, make([]byte, 32), crypto.SHA256)
	assert.ErrorContains(t, err, "closed")
}

func TestInitializeRequestEncoding(t *testing.T) {
	req := newRequest(callInitialize)
	req.byteArray([]byte(rpcHandshake))
	req.byte(0)
	req.byteArray([]byte{0})

	want := []byte{0, 0, 0, 1, 0, 0, 0, 5}
	want = append(want, "ayyay"...)
	want = append(want, 1, 0, 0, 0, byte(len(rpcHandshake)))
	want = append(want, rpcHandshake...)
	want = append(want, 0)
	want = append(want, 1, 0, 0, 0, 1, 0)
	assert.Equal(t, want, req.b)
}

func TestMechanismEncoding(t *testing.T) {
	for _, tt := range []struct {
		name string
		m    mechanism
		want []byte
	}{
		{
			name: "rsa pkcs without parameters",
			m:    mechanism{typ: ckmRSAPKCS},
			want: []byte{0, 0, 0, 1},
		},
		{
			name: "ecdsa without parameters",
			m:    mechanism{typ: ckmECDSA},
			want: []byte{0, 0, 0x10, 0x41},
		},
		{
			name: "rsa pss with parameters",
			m:    mechanism{typ: ckmRSAPKCSPSS, pss: &pssParams{hashAlg: ckmSHA256, mgf: ckgMGF1SHA256, sLen: 32}},
			want: []byte{
				0, 0, 0, 0x0d, 1,
				0, 0, 0, 0, 0, 0, 0x02, 0x50,
				0, 0, 0, 0, 0, 0, 0, 2,
				0, 0, 0, 0, 0, 0, 0, 32,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := &encoder{}
			e.mechanism(tt.m)
			assert.Equal(t, tt.want, e.b)

			d := &decoder{b: e.b}
			assert.Equal(t, tt.m, d.mechanism())
			assert.NoError(t, d.done())
		})
	}
}

func TestAttributeEncoding(t *testing.T) {
	e := &encoder{}
	e.attributes([]attribute{ulongAttribute(ckaClass, ckoPrivateKey), bytesAttribute(ckaLabel, []byte("k"))})
	want := []byte{
		0, 0, 0, 2,
		0, 0, 0, 0, 1, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 3,
		0, 0, 0, 3, 1, 0, 0, 0, 1, 0, 0, 0, 1, 'k',
	}
	assert.Equal(t, want, e.b)
}

func TestParseResponse(t *testing.T) {
	for _, tt := range []struct {
		name    string
		call    uint32
		body    []byte
		wantErr error
	}{
		{
			name:    "pkcs11 error",
			call:    callLogin,
			body:    errorResponse(ckrPinIncorrect),
			wantErr: ckError(ckrPinIncorrect),
		},
		{
			name:    "unexpected call",
			call:    callLogin,
			body:    newRequest(callFinalize).b,
			wantErr: errProtocol,
		},
		{
			name:    "unexpected signature",
			call:    callFinalize,
			body:    append([]byte{0, 0, 0, 2}, 0, 0, 0, 1, 'u'),
			wantErr: errProtocol,
		},
		{
			name:    "short message",
			call:    callFinalize,
			body:    []byte{0, 0},
			wantErr: errProtocol,
		},
		{
			name:    "error without code",
			call:    callFinalize,
			body:    errorResponse(ckrOK),
			wantErr: errProtocol,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseResponse(tt.call, tt.body)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestReadFrameErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header []byte
	}{
		{name: "zero code", header: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{name: "too large", header: []byte{0, 0, 0, 1, 0, 0, 0, 0, 0xff, 0, 0, 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := readFrame(bytesReader(tt.header))
			assert.ErrorIs(t, err, errProtocol)
		})
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}

func BenchmarkSign(b *testing.B) {
	for _, keyType := range []string{"rsa", "p256"} {
		b.Run(keyType, func(b *testing.B) {
			key := newKey(b, keyType)
			f := newFakeServer(&fakeToken{label: "acm", keys: []fakeKey{{signer: key}}})
			defer f.close()

			o := testOptions(f)
			o.SessionsPerToken = 8
			s, err := newSigner(o, Certificate{TokenLabel: "acm"}, nil, key.Public())
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()

			digest := sha256.Sum256([]byte("data"))
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := s.Sign(nil, digest[:], crypto.SHA256); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}
