//go:build awsnitro && linux

package nitro

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/secrets/certregistry"
)

func TestMain(m *testing.M) {
	os.Exit(noleak.CheckMain(m))
}

type (
	mockObject struct {
		handle   uint64
		class    uint64
		id       byte
		category uint64
		der      []byte
		rsaKey   *rsa.PrivateKey
		ecKey    *ecdsa.PrivateKey
	}

	mockToken struct {
		slotID  uint64
		label   string
		pin     string
		objects []mockObject
	}

	mockSession struct {
		slotID     uint64
		loggedIn   bool
		findActive bool
		findResult []uint64
		findIndex  int
		signMech   p11Mechanism
		signKey    uint64
	}

	mockEnclaveServer struct {
		mu          sync.Mutex
		tokens      map[uint64]*mockToken
		sessions    map[uint64]*mockSession
		nextSession uint64
		conns       []net.Conn
	}
)

func newMockEnclaveServer(tokens ...*mockToken) *mockEnclaveServer {
	tokMap := make(map[uint64]*mockToken, len(tokens))
	for _, t := range tokens {
		tokMap[t.slotID] = t
	}
	return &mockEnclaveServer{
		tokens:      tokMap,
		sessions:    make(map[uint64]*mockSession),
		nextSession: 1,
	}
}

func (s *mockEnclaveServer) updateToken(tok *mockToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tok.slotID] = tok
}

func (s *mockEnclaveServer) dial(_ uint32, _ uint32) (io.ReadWriteCloser, error) {
	c1, c2 := net.Pipe()
	s.mu.Lock()
	s.conns = append(s.conns, c1, c2)
	s.mu.Unlock()
	go s.serveConn(c2)
	return c1, nil
}

func (s *mockEnclaveServer) closeAll() {
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (s *mockEnclaveServer) serveConn(conn net.Conn) {
	defer conn.Close()

	var ver [1]byte
	if _, err := io.ReadFull(conn, ver[:]); err != nil {
		return
	}
	if _, err := conn.Write(ver[:]); err != nil {
		return
	}

	var hdr [12]byte
	for {
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		callCode := binary.BigEndian.Uint32(hdr[0:4])
		olen := binary.BigEndian.Uint32(hdr[4:8])
		dlen := binary.BigEndian.Uint32(hdr[8:12])

		if olen > 0 {
			opts := make([]byte, olen)
			if _, err := io.ReadFull(conn, opts); err != nil {
				return
			}
		}
		payload := make([]byte, dlen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}

		respPayload := s.handleCall(&rpcBuf{b: payload})

		binary.BigEndian.PutUint32(hdr[0:4], callCode)
		binary.BigEndian.PutUint32(hdr[4:8], 0)
		binary.BigEndian.PutUint32(hdr[8:12], uint32(len(respPayload)))
		if _, err := conn.Write(hdr[:]); err != nil {
			return
		}
		if _, err := conn.Write(respPayload); err != nil {
			return
		}
	}
}

func writeRPCError(code uint64) []byte {
	var b rpcBuf
	b.writeUint32(p11CallError)
	b.writeBytes([]byte("u"))
	b.writeUint64(code)
	return b.b
}

func writeRPCOK(callID uint32, sig string, fn func(*rpcBuf)) []byte {
	var b rpcBuf
	b.writeUint32(callID)
	b.writeBytes([]byte(sig))
	if fn != nil {
		fn(&b)
	}
	return b.b
}

func padRight(str string, length int) []byte {
	out := make([]byte, length)
	for i := range out {
		out[i] = ' '
	}
	copy(out, []byte(str))
	return out
}

func (s *mockEnclaveServer) handleCall(req *rpcBuf) []byte {
	callID, err := req.readUint32()
	if err != nil {
		return writeRPCError(0x00000006)
	}
	_, err = req.readBytes()
	if err != nil {
		return writeRPCError(0x00000006)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch callID {
	case p11CallInitialize, p11CallFinalize:
		return writeRPCOK(callID, "", nil)

	case p11CallGetSlotList:
		_, _ = req.readByte()
		bufCount, _ := req.readUint32()
		slots := make([]uint64, 0, len(s.tokens))
		for id := range s.tokens {
			slots = append(slots, id)
		}
		return writeRPCOK(callID, "au", func(b *rpcBuf) {
			if bufCount == 0 {
				b.writeByte(0)
				b.writeUint32(uint32(len(slots)))
				return
			}
			b.writeByte(1)
			b.writeUint32(uint32(len(slots)))
			for _, id := range slots {
				b.writeUint64(id)
			}
		})

	case p11CallGetTokenInfo:
		slotID, _ := req.readUint64()
		tok, ok := s.tokens[slotID]
		if !ok {
			return writeRPCError(0x00000003)
		}
		return writeRPCOK(callID, "ssssuuuuuuuuuuuvvs", func(b *rpcBuf) {
			b.writeBytes(padRight(tok.label, 32))
			b.writeBytes(padRight("Amazon", 32))
			b.writeBytes(padRight("p11ne-token", 16))
			b.writeBytes(padRight("EVT00", 16))
			b.writeUint64(ckfLoginRequired)
			b.writeUint64(1024)
			for range 9 {
				b.writeUint64(0)
			}
			b.writeByte(0)
			b.writeByte(1)
			b.writeByte(0)
			b.writeByte(1)
			b.writeBytes(padRight("", 16))
		})

	case p11CallOpenSession:
		slotID, _ := req.readUint64()
		_, _ = req.readUint64()
		if _, ok := s.tokens[slotID]; !ok {
			return writeRPCError(0x00000003)
		}
		h := s.nextSession
		s.nextSession++
		s.sessions[h] = &mockSession{slotID: slotID}
		return writeRPCOK(callID, "u", func(b *rpcBuf) {
			b.writeUint64(h)
		})

	case p11CallCloseSession:
		h, _ := req.readUint64()
		delete(s.sessions, h)
		return writeRPCOK(callID, "", nil)

	case p11CallLogin:
		h, _ := req.readUint64()
		_, _ = req.readUint64()
		pinBytes, _, _ := req.readByteArray()
		sess, ok := s.sessions[h]
		if !ok {
			return writeRPCError(0x000000B3)
		}
		tok := s.tokens[sess.slotID]
		if string(pinBytes) != tok.pin {
			return writeRPCError(0x000000A0)
		}
		sess.loggedIn = true
		return writeRPCOK(callID, "", nil)

	case p11CallFindObjectsInit:
		h, _ := req.readUint64()
		sess, ok := s.sessions[h]
		if !ok {
			return writeRPCError(0x000000B3)
		}
		tok := s.tokens[sess.slotID]
		numAttrs, _ := req.readUint32()
		var wantClass uint64
		var filterClass bool
		for range numAttrs {
			t, _ := req.readUint32()
			valid, _ := req.readByte()
			if valid != 0 {
				_, _ = req.readUint32()
				if t == ckaClass {
					wantClass, _ = req.readUint64()
					filterClass = true
				}
			}
		}
		sess.findResult = nil
		sess.findIndex = 0
		sess.findActive = true
		for _, obj := range tok.objects {
			if !filterClass || obj.class == wantClass {
				sess.findResult = append(sess.findResult, obj.handle)
			}
		}
		return writeRPCOK(callID, "", nil)

	case p11CallFindObjects:
		h, _ := req.readUint64()
		maxCount, _ := req.readUint32()
		sess, ok := s.sessions[h]
		if !ok || !sess.findActive {
			return writeRPCError(0x00000091)
		}
		end := min(sess.findIndex+int(maxCount), len(sess.findResult))
		chunk := sess.findResult[sess.findIndex:end]
		sess.findIndex = end
		return writeRPCOK(callID, "au", func(b *rpcBuf) {
			b.writeByte(1)
			b.writeUint32(uint32(len(chunk)))
			for _, objH := range chunk {
				b.writeUint64(objH)
			}
		})

	case p11CallFindObjectsFinal:
		h, _ := req.readUint64()
		if sess, ok := s.sessions[h]; ok {
			sess.findActive = false
		}
		return writeRPCOK(callID, "", nil)

	case p11CallGetAttributeValue:
		h, _ := req.readUint64()
		objH, _ := req.readUint64()
		count, _ := req.readUint32()
		sess, ok := s.sessions[h]
		if !ok {
			return writeRPCError(0x000000B3)
		}
		tok := s.tokens[sess.slotID]
		var target *mockObject
		for i := range tok.objects {
			if tok.objects[i].handle == objH {
				target = &tok.objects[i]
				break
			}
		}
		if target == nil {
			return writeRPCError(0x00000082)
		}

		reqTypes := make([]uint32, count)
		for i := range count {
			reqTypes[i], _ = req.readUint32()
			_, _ = req.readUint32()
		}

		return writeRPCOK(callID, "aAu", func(b *rpcBuf) {
			var attrs []p11Attribute
			for _, t := range reqTypes {
				switch t {
				case ckaValue:
					attrs = append(attrs, p11Attribute{Type: t, Value: target.der, Valid: true})
				case ckaID:
					attrs = append(attrs, p11Attribute{Type: t, Value: []byte{target.id}, Valid: true})
				case ckaCertificateCategory:
					attrs = append(attrs, p11Attribute{Type: t, ULong: target.category, Valid: true})
				default:
					attrs = append(attrs, p11Attribute{Type: t, Valid: false})
				}
			}
			b.writeAttributeArray(attrs)
			b.writeUint64(ckrOK)
		})

	case p11CallSignInit:
		h, _ := req.readUint64()
		mechType, _ := req.readUint32()
		mech := p11Mechanism{Type: mechType}
		if mechType == ckmRSAPKCSPSS {
			hasParam, _ := req.readByte()
			if hasParam != 0 {
				hAlg, _ := req.readUint64()
				mgf, _ := req.readUint64()
				sLen, _ := req.readUint64()
				mech.PSSParams = &p11RSAPSSParams{HashAlg: hAlg, MGF: mgf, SLen: sLen}
			}
		}
		keyH, _ := req.readUint64()
		sess, ok := s.sessions[h]
		if !ok || !sess.loggedIn {
			return writeRPCError(0x00000101)
		}
		sess.signMech = mech
		sess.signKey = keyH
		return writeRPCOK(callID, "", nil)

	case p11CallSign:
		h, _ := req.readUint64()
		data, _, _ := req.readByteArray()
		_, _ = req.readUint32()
		sess, ok := s.sessions[h]
		if !ok || !sess.loggedIn {
			return writeRPCError(0x00000101)
		}
		tok := s.tokens[sess.slotID]
		var keyObj *mockObject
		for i := range tok.objects {
			if tok.objects[i].handle == sess.signKey {
				keyObj = &tok.objects[i]
				break
			}
		}
		if keyObj == nil {
			return writeRPCError(0x00000060)
		}

		sig, err := performMockSign(keyObj, sess.signMech, data)
		if err != nil {
			return writeRPCError(0x00000005)
		}
		return writeRPCOK(callID, "ay", func(b *rpcBuf) {
			b.writeByteArray(sig)
		})

	default:
		return writeRPCError(0x00000054)
	}
}

func performMockSign(keyObj *mockObject, mech p11Mechanism, data []byte) ([]byte, error) {
	switch mech.Type {
	case ckmRSAPKCSPSS:
		if keyObj.rsaKey == nil || mech.PSSParams == nil {
			return nil, fmt.Errorf("missing rsa key or pss params")
		}
		var h crypto.Hash
		switch mech.PSSParams.HashAlg {
		case ckmSHA256:
			h = crypto.SHA256
		case ckmSHA384:
			h = crypto.SHA384
		case ckmSHA512:
			h = crypto.SHA512
		default:
			return nil, fmt.Errorf("unsupported hash alg: %x", mech.PSSParams.HashAlg)
		}
		return rsa.SignPSS(rand.Reader, keyObj.rsaKey, h, data, &rsa.PSSOptions{
			SaltLength: int(mech.PSSParams.SLen),
			Hash:       h,
		})

	case ckmRSAPKCS:
		if keyObj.rsaKey == nil {
			return nil, fmt.Errorf("missing rsa key")
		}
		for h, prefix := range hashPrefixes {
			if len(data) > len(prefix) && string(data[:len(prefix)]) == string(prefix) {
				return rsa.SignPKCS1v15(rand.Reader, keyObj.rsaKey, h, data[len(prefix):])
			}
		}
		return rsa.SignPKCS1v15(rand.Reader, keyObj.rsaKey, crypto.Hash(0), data)

	case ckmECDSA:
		if keyObj.ecKey == nil {
			return nil, fmt.Errorf("missing ec key")
		}
		r, s, err := ecdsa.Sign(rand.Reader, keyObj.ecKey, data)
		if err != nil {
			return nil, err
		}
		byteLen := (keyObj.ecKey.Curve.Params().BitSize + 7) / 8
		out := make([]byte, byteLen*2)
		rBytes := r.Bytes()
		sBytes := s.Bytes()
		copy(out[byteLen-len(rBytes):byteLen], rBytes)
		copy(out[byteLen*2-len(sBytes):], sBytes)
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported mechanism: %x", mech.Type)
	}
}

func generateTestRSAKeyAndCert(t *testing.T, dnsName string, notBefore time.Time) (*rsa.PrivateKey, []byte, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: dnsName},
		DNSNames:              []string{dnsName},
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return priv, der, parsed
}

func generateTestECKeyAndCert(t *testing.T, dnsName string, notBefore time.Time) (*ecdsa.PrivateKey, []byte, *x509.Certificate) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: dnsName},
		DNSNames:              []string{dnsName},
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return priv, der, parsed
}

func TestParseModuleAndStanza(t *testing.T) {
	for _, tt := range []struct {
		name     string
		module   string
		wantCID  uint32
		wantPort uint32
		wantErr  bool
	}{
		{
			name:     "valid module file",
			module:   "remote:vsock:cid=16;port=9999\nmodule:p11ne\n",
			wantCID:  16,
			wantPort: 9999,
		},
		{
			name:     "valid module file default port",
			module:   "remote:vsock:cid=42\nmodule:p11ne\n",
			wantCID:  42,
			wantPort: DefaultEnclavePort,
		},
		{
			name:    "invalid module file",
			module:  "module:p11ne\n",
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cid, port, err := parseModuleContent(tt.module)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseModuleContent() err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (cid != tt.wantCID || port != tt.wantPort) {
				t.Fatalf("got cid=%d port=%d, want cid=%d port=%d", cid, port, tt.wantCID, tt.wantPort)
			}
		})
	}

	stanzas := make(map[string]tokenStanzaConfig)
	conf := `
ssl_certificate_key "engine:pkcs11:pkcs11:model=p11ne-token;manufacturer=Amazon;token=skipper-acm-token;id=%01;object=acm-key;type=private?pin-value=mysecretpin";
ssl_certificate "/run/nitro_enclaves/acm/skipper-cert.pem";
`
	parseStanzaContent(conf, stanzas)
	st, ok := stanzas["skipper-acm-token"]
	if !ok {
		t.Fatal("expected skipper-acm-token in parsed stanzas")
	}
	if st.PIN != "mysecretpin" {
		t.Fatalf("got PIN %q, want %q", st.PIN, "mysecretpin")
	}
	if st.CertPath != "/run/nitro_enclaves/acm/skipper-cert.pem" {
		t.Fatalf("got CertPath %q, want %q", st.CertPath, "/run/nitro_enclaves/acm/skipper-cert.pem")
	}
}

func TestNitroProxyTLSIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "p11ne.module")
	if err := os.WriteFile(modulePath, []byte("remote:vsock:cid=16;port=9999\nmodule:p11ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	confPath := filepath.Join(tmpDir, "skipper-acm.conf")
	stanza := `ssl_certificate_key "engine:pkcs11:pkcs11:model=p11ne-token;manufacturer=Amazon;token=skipper-token-rsa;id=%01;object=acm-key;type=private?pin-value=secretpin123";` + "\n"
	if err := os.WriteFile(confPath, []byte(stanza), 0o600); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now().Add(-time.Hour)
	rsaKey, rsaDER, rsaCert := generateTestRSAKeyAndCert(t, "rsa.nitro.example.test", t0)
	ecKey, ecDER, ecCert := generateTestECKeyAndCert(t, "ec.nitro.example.test", t0)

	enclave := newMockEnclaveServer(
		&mockToken{
			slotID: 0,
			label:  "skipper-token-rsa",
			pin:    "secretpin123",
			objects: []mockObject{
				{
					handle:   1,
					class:    ckoCertificate,
					id:       1,
					category: ckCertCategoryTokenUser,
					der:      rsaDER,
				},
				{
					handle: 2,
					class:  ckoPrivateKey,
					id:     1,
					rsaKey: rsaKey,
				},
			},
		},
		&mockToken{
			slotID: 1,
			label:  "skipper-token-ec",
			pin:    "secretpin123",
			objects: []mockObject{
				{
					handle:   1,
					class:    ckoCertificate,
					id:       1,
					category: ckCertCategoryTokenUser,
					der:      ecDER,
				},
				{
					handle: 2,
					class:  ckoPrivateKey,
					id:     1,
					ecKey:  ecKey,
				},
			},
		},
	)
	defer enclave.closeAll()

	cr := certregistry.NewCertRegistry()
	provider, err := NewProvider(Options{
		ModulePath:      modulePath,
		ConfPath:        filepath.Join(tmpDir, "*.conf"),
		RefreshInterval: 50 * time.Millisecond,
		Dial:            enclave.dial,
	}, cr)
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	defer provider.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Host", r.Host)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello-from-nitro-enclave"))
	}))
	defer backend.Close()

	fr := make(filters.Registry)
	routes := eskip.MustParse(fmt.Sprintf(`* -> "%s"`, backend.URL))
	dc := testdataclient.New(routes)
	defer dc.Close()
	rt := routing.New(routing.Options{
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		SignalFirstLoad: true,
	})
	defer rt.Close()
	<-rt.FirstLoad()

	prx := proxy.WithParams(proxy.Params{
		Routing:              rt,
		CloseIdleConnsPeriod: -time.Second,
	})
	defer prx.Close()

	tlsLn, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		GetCertificate: cr.GetCertFromHello,
		MinVersion:     tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tlsLn.Close()

	srv := &http.Server{Handler: prx}
	go func() {
		_ = srv.Serve(tlsLn)
	}()
	defer srv.Close()

	caPool := x509.NewCertPool()
	caPool.AddCert(rsaCert)
	caPool.AddCert(ecCert)

	for _, tc := range []struct {
		name       string
		serverName string
		tlsVersion uint16
	}{
		{
			name:       "TLS 1.3 with RSA-PSS offload via vsock enclave",
			serverName: "rsa.nitro.example.test",
			tlsVersion: tls.VersionTLS13,
		},
		{
			name:       "TLS 1.2 with RSA offload via vsock enclave",
			serverName: "rsa.nitro.example.test",
			tlsVersion: tls.VersionTLS12,
		},
		{
			name:       "TLS 1.3 with ECDSA P-256 offload via vsock enclave",
			serverName: "ec.nitro.example.test",
			tlsVersion: tls.VersionTLS13,
		},
		{
			name:       "TLS 1.2 with ECDSA P-256 offload via vsock enclave",
			serverName: "ec.nitro.example.test",
			tlsVersion: tls.VersionTLS12,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:    caPool,
					ServerName: tc.serverName,
					MinVersion: tc.tlsVersion,
					MaxVersion: tc.tlsVersion,
				},
			}
			defer tr.CloseIdleConnections()

			client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/test", tlsLn.Addr().String()), nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = tc.serverName

			rsp, err := client.Do(req)
			if err != nil {
				t.Fatalf("HTTPS request failed: %v", err)
			}
			defer rsp.Body.Close()

			body, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("got status %d, want 200", rsp.StatusCode)
			}
			if string(body) != "hello-from-nitro-enclave" {
				t.Fatalf("unexpected body: %q", string(body))
			}
		})
	}

	rotatedKey, rotatedDER, rotatedCert := generateTestRSAKeyAndCert(t, "rsa.nitro.example.test", time.Now())
	enclave.updateToken(&mockToken{
		slotID: 0,
		label:  "skipper-token-rsa",
		pin:    "secretpin123",
		objects: []mockObject{
			{
				handle:   1,
				class:    ckoCertificate,
				id:       1,
				category: ckCertCategoryTokenUser,
				der:      rotatedDER,
			},
			{
				handle: 2,
				class:  ckoPrivateKey,
				id:     1,
				rsaKey: rotatedKey,
			},
		},
	})

	rotatedPool := x509.NewCertPool()
	rotatedPool.AddCert(rotatedCert)

	deadline := time.Now().Add(2 * time.Second)
	for {
		c, err := cr.GetCertFromHello(&tls.ClientHelloInfo{ServerName: "rsa.nitro.example.test"})
		if err == nil && c != nil && c.Leaf != nil && c.Leaf.SerialNumber.Cmp(rotatedCert.SerialNumber) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for enclave certificate rotation")
		}
		time.Sleep(25 * time.Millisecond)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    rotatedPool,
			ServerName: "rsa.nitro.example.test",
		},
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	rsp, err := client.Get(fmt.Sprintf("https://%s/rotated", tlsLn.Addr().String()))
	if err != nil {
		t.Fatalf("HTTPS request with rotated cert failed: %v", err)
	}
	rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", rsp.StatusCode)
	}
}

func TestSignerPKCS1v15AndDecryptAndFallbackPEM(t *testing.T) {
	tmpDir := t.TempDir()
	rsaKey, rsaDER, rsaCert := generateTestRSAKeyAndCert(t, "pem.nitro.example.test", time.Now().Add(-time.Hour))

	pemPath := filepath.Join(tmpDir, "skipper-cert.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rsaDER})
	if err := os.WriteFile(pemPath, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	confPath := filepath.Join(tmpDir, "skipper-acm.conf")
	stanza := fmt.Sprintf("ssl_certificate_key \"engine:pkcs11:pkcs11:token=pem-token?pin-value=pin456\";\nssl_certificate %q;\n", pemPath)
	if err := os.WriteFile(confPath, []byte(stanza), 0o600); err != nil {
		t.Fatal(err)
	}

	enclave := newMockEnclaveServer(&mockToken{
		slotID: 0,
		label:  "pem-token",
		pin:    "pin456",
		objects: []mockObject{
			{
				handle: 1,
				class:  ckoPrivateKey,
				id:     1,
				rsaKey: rsaKey,
			},
		},
	})
	defer enclave.closeAll()

	cr := certregistry.NewCertRegistry()
	provider, err := NewProvider(Options{
		EnclaveCID:      16,
		EnclavePort:     9999,
		ConfPath:        filepath.Join(tmpDir, "*.conf"),
		RefreshInterval: time.Hour,
		Dial:            enclave.dial,
	}, cr)
	if err != nil {
		t.Fatalf("NewProvider failed: %v", err)
	}
	defer provider.Close()

	tlsCert, err := cr.GetCertFromHello(&tls.ClientHelloInfo{ServerName: "pem.nitro.example.test"})
	if err != nil || tlsCert == nil {
		t.Fatalf("expected certificate from registry, got err=%v", err)
	}

	signer, ok := tlsCert.PrivateKey.(crypto.Signer)
	if !ok {
		t.Fatalf("expected crypto.Signer, got %T", tlsCert.PrivateKey)
	}
	digest := sha256.Sum256([]byte("test message"))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("PKCS1v15 Sign failed: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(rsaCert.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("VerifyPKCS1v15 failed: %v", err)
	}
}

func TestProviderErrorCases(t *testing.T) {
	cr := certregistry.NewCertRegistry()

	for _, tt := range []struct {
		name string
		opts Options
		cr   *certregistry.CertRegistry
	}{
		{
			name: "nil certregistry",
			opts: Options{EnclaveCID: 16},
			cr:   nil,
		},
		{
			name: "missing module file when CID is 0",
			opts: Options{ModulePath: "/nonexistent/path/p11ne.module"},
			cr:   cr,
		},
		{
			name: "dial failure",
			opts: Options{
				EnclaveCID: 16,
				Dial: func(cid, port uint32) (io.ReadWriteCloser, error) {
					return nil, fmt.Errorf("dial failed")
				},
			},
			cr: cr,
		},
		{
			name: "empty slots in enclave",
			opts: Options{
				EnclaveCID: 16,
				Dial:       newMockEnclaveServer().dial,
			},
			cr: cr,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider(tt.opts, tt.cr)
			if err == nil {
				p.Close()
				t.Fatal("expected error, got nil")
			}
		})
	}
}
