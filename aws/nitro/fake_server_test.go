//go:build awsnitro

package nitro

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeP11Object struct {
	handle   uint64
	class    uint64
	certType uint64
	id       []byte
	value    []byte
	signer   crypto.Signer
}

type fakeSession struct {
	slotID      uint64
	findMatches []uint64
	findIdx     int
	signMech    p11Mechanism
	signKey     uint64
}

type fakeP11Server struct {
	ln       net.Listener
	endpoint string

	mu          sync.Mutex
	tokenLabel  string
	objects     map[uint64]*fakeP11Object
	nextObj     uint64
	nextSession uint64
	sessions    map[uint64]*fakeSession
	conns       map[net.Conn]struct{}
	dropNext    atomic.Bool

	wg sync.WaitGroup
}

func startFakeP11Server(t *testing.T, tokenLabel string) *fakeP11Server {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "p11.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	s := &fakeP11Server{
		ln:          ln,
		endpoint:    "unix://" + sockPath,
		tokenLabel:  tokenLabel,
		objects:     make(map[uint64]*fakeP11Object),
		nextObj:     1,
		nextSession: 100,
		sessions:    make(map[uint64]*fakeSession),
		conns:       make(map[net.Conn]struct{}),
	}
	s.wg.Add(1)
	go s.acceptLoop()
	t.Cleanup(func() {
		s.close()
		_ = os.Remove(sockPath)
	})
	return s
}

func (s *fakeP11Server) close() {
	_ = s.ln.Close()
	s.mu.Lock()
	for c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *fakeP11Server) addCertAndKey(id []byte, leafDER []byte, key crypto.Signer, intermediates ...[]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h := s.nextObj
	s.nextObj++
	s.objects[h] = &fakeP11Object{
		handle:   h,
		class:    ckoCertificate,
		certType: ckcX509,
		id:       append([]byte(nil), id...),
		value:    append([]byte(nil), leafDER...),
	}

	for _, inter := range intermediates {
		ih := s.nextObj
		s.nextObj++
		s.objects[ih] = &fakeP11Object{
			handle:   ih,
			class:    ckoCertificate,
			certType: ckcX509,
			id:       append([]byte(nil), id...),
			value:    append([]byte(nil), inter...),
		}
	}

	kh := s.nextObj
	s.nextObj++
	s.objects[kh] = &fakeP11Object{
		handle: kh,
		class:  ckoPrivateKey,
		id:     append([]byte(nil), id...),
		signer: key,
	}
}

func (s *fakeP11Server) resetObjects() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects = make(map[uint64]*fakeP11Object)
}

func (s *fakeP11Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *fakeP11Server) serveConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	var ver [1]byte
	if _, err := io.ReadFull(conn, ver[:]); err != nil {
		return
	}
	if _, err := conn.Write(ver[:]); err != nil {
		return
	}

	for {
		var hdr [12]byte
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		code := binary.BigEndian.Uint32(hdr[0:4])
		optLen := binary.BigEndian.Uint32(hdr[4:8])
		bufLen := binary.BigEndian.Uint32(hdr[8:12])
		if optLen > 0 {
			if _, err := io.CopyN(io.Discard, conn, int64(optLen)); err != nil {
				return
			}
		}
		payload := make([]byte, bufLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}

		req := &rpcBuf{data: payload}
		callID, err := req.readUint32()
		if err != nil {
			return
		}
		if _, err := req.readRawByteArray(); err != nil {
			return
		}

		if callID == rpcCallCSign && s.dropNext.Swap(false) {
			return
		}

		resp := s.handleCall(callID, req)
		binary.BigEndian.PutUint32(hdr[0:4], code)
		binary.BigEndian.PutUint32(hdr[4:8], 0)
		binary.BigEndian.PutUint32(hdr[8:12], uint32(len(resp.data)))
		if err := writeAll(conn, hdr[:]); err != nil {
			return
		}
		if err := writeAll(conn, resp.data); err != nil {
			return
		}
	}
}

func (s *fakeP11Server) handleCall(callID uint32, req *rpcBuf) *rpcBuf {
	switch callID {
	case rpcCallCInitialize:
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCInitialize, "")
		return resp

	case rpcCallCFinalize:
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCFinalize, "")
		return resp

	case rpcCallCGetSlotList:
		_, _ = req.readByte()
		maxCount, _ := req.readUint32()
		resp := newRPCBuf(32)
		resp.writeHeader(rpcCallCGetSlotList, "au")
		if maxCount < 1 {
			resp.writeByte(0)
			resp.writeUint32(1)
			return resp
		}
		resp.writeByte(1)
		resp.writeUint32(1)
		resp.writeUint64(1)
		return resp

	case rpcCallCGetTokenInfo:
		slotID, _ := req.readUint64()
		if slotID != 1 {
			return errorResp(0x00000003)
		}
		resp := newRPCBuf(256)
		resp.writeHeader(rpcCallCGetTokenInfo, "ssssuuuuuuuuuuuvvs")
		resp.writeRawByteArray(spacePad(s.tokenLabel, 32))
		resp.writeRawByteArray(spacePad("AWS", 32))
		resp.writeRawByteArray(spacePad("Nitro", 16))
		resp.writeRawByteArray(spacePad("0001", 16))
		for range 11 {
			resp.writeUint64(0)
		}
		resp.writeByte(2)
		resp.writeByte(40)
		resp.writeByte(1)
		resp.writeByte(0)
		resp.writeRawByteArray(spacePad("", 16))
		return resp

	case rpcCallCOpenSession:
		slotID, _ := req.readUint64()
		_, _ = req.readUint64()
		s.mu.Lock()
		sid := s.nextSession
		s.nextSession++
		s.sessions[sid] = &fakeSession{slotID: slotID}
		s.mu.Unlock()
		resp := newRPCBuf(24)
		resp.writeHeader(rpcCallCOpenSession, "u")
		resp.writeUint64(sid)
		return resp

	case rpcCallCCloseSession:
		sid, _ := req.readUint64()
		s.mu.Lock()
		delete(s.sessions, sid)
		s.mu.Unlock()
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCCloseSession, "")
		return resp

	case rpcCallCFindObjectsInit:
		sid, _ := req.readUint64()
		tmpl := readTemplate(req)
		s.mu.Lock()
		sess := s.sessions[sid]
		if sess == nil {
			s.mu.Unlock()
			return errorResp(ckrSessionHandleInvalid)
		}
		var matches []uint64
		for _, obj := range s.objects {
			if matchTemplate(obj, tmpl) {
				matches = append(matches, obj.handle)
			}
		}
		sess.findMatches = matches
		sess.findIdx = 0
		s.mu.Unlock()
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCFindObjectsInit, "")
		return resp

	case rpcCallCFindObjects:
		sid, _ := req.readUint64()
		maxCount, _ := req.readUint32()
		s.mu.Lock()
		sess := s.sessions[sid]
		if sess == nil {
			s.mu.Unlock()
			return errorResp(ckrSessionHandleInvalid)
		}
		rem := sess.findMatches[sess.findIdx:]
		if uint32(len(rem)) > maxCount {
			rem = rem[:maxCount]
		}
		sess.findIdx += len(rem)
		s.mu.Unlock()

		resp := newRPCBuf(32 + len(rem)*8)
		resp.writeHeader(rpcCallCFindObjects, "au")
		resp.writeByte(1)
		resp.writeUint32(uint32(len(rem)))
		for _, h := range rem {
			resp.writeUint64(h)
		}
		return resp

	case rpcCallCFindObjectsFinal:
		sid, _ := req.readUint64()
		s.mu.Lock()
		if sess := s.sessions[sid]; sess != nil {
			sess.findMatches = nil
			sess.findIdx = 0
		}
		s.mu.Unlock()
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCFindObjectsFinal, "")
		return resp

	case rpcCallCGetAttributeVal:
		sid, _ := req.readUint64()
		objHandle, _ := req.readUint64()
		count, _ := req.readUint32()
		reqs := make([]p11AttrRequest, count)
		for i := range reqs {
			reqs[i].typ, _ = req.readUint32()
			reqs[i].bufLen, _ = req.readUint32()
		}
		s.mu.Lock()
		sess := s.sessions[sid]
		obj := s.objects[objHandle]
		s.mu.Unlock()
		if sess == nil {
			return errorResp(ckrSessionHandleInvalid)
		}
		if obj == nil {
			return errorResp(0x00000082)
		}

		attrs := make([]p11Attribute, len(reqs))
		rv := ckrOK
		for i, r := range reqs {
			switch r.typ {
			case ckaValue:
				attrs[i] = attrBytes(ckaValue, obj.value)
			case ckaID:
				attrs[i] = attrBytes(ckaID, obj.id)
			case ckaClass:
				attrs[i] = attrUlong(ckaClass, obj.class)
			default:
				attrs[i] = p11Attribute{typ: r.typ, valid: false}
				rv = ckrAttributeTypeInvalid
			}
		}
		resp := newRPCBuf(128)
		resp.writeHeader(rpcCallCGetAttributeVal, "aAu")
		writeAttributeArray(resp, attrs)
		resp.writeUint64(rv)
		return resp

	case rpcCallCSignInit:
		sid, _ := req.readUint64()
		mech := readMechanism(req)
		keyHandle, _ := req.readUint64()
		s.mu.Lock()
		sess := s.sessions[sid]
		if sess == nil {
			s.mu.Unlock()
			return errorResp(ckrSessionHandleInvalid)
		}
		sess.signMech = mech
		sess.signKey = keyHandle
		s.mu.Unlock()
		resp := newRPCBuf(16)
		resp.writeHeader(rpcCallCSignInit, "")
		return resp

	case rpcCallCSign:
		sid, _ := req.readUint64()
		data, _, _, _ := req.readByteArray()
		maxSigLen, _ := req.readUint32()
		s.mu.Lock()
		sess := s.sessions[sid]
		if sess == nil {
			s.mu.Unlock()
			return errorResp(ckrSessionHandleInvalid)
		}
		mech := sess.signMech
		obj := s.objects[sess.signKey]
		s.mu.Unlock()
		if obj == nil || obj.signer == nil {
			return errorResp(0x00000060)
		}

		sig, err := fakeSign(obj.signer, mech, data)
		if err != nil {
			return errorResp(0x00000005)
		}
		resp := newRPCBuf(32 + len(sig))
		resp.writeHeader(rpcCallCSign, "ay")
		if uint32(len(sig)) > maxSigLen {
			resp.writeByte(0)
			resp.writeUint32(uint32(len(sig)))
			return resp
		}
		resp.writeByteArray(sig)
		return resp

	default:
		return errorResp(0x00000054)
	}
}

func errorResp(rv uint64) *rpcBuf {
	resp := newRPCBuf(24)
	resp.writeHeader(rpcCallError, "u")
	resp.writeUint64(rv)
	return resp
}

func spacePad(s string, n int) []byte {
	out := bytes.Repeat([]byte(" "), n)
	copy(out, s)
	return out
}

func readTemplate(b *rpcBuf) []p11Attribute {
	n, _ := b.readUint32()
	out := make([]p11Attribute, n)
	for i := range out {
		typ, _ := b.readUint32()
		valid, _ := b.readByte()
		out[i].typ = typ
		if valid == 0 {
			continue
		}
		out[i].valid = true
		_, _ = b.readUint32()
		switch attrValueKind(typ) {
		case p11ValByte:
			out[i].byteVal, _ = b.readByte()
		case p11ValUlong:
			out[i].ulong, _ = b.readUint64()
		default:
			out[i].bytes, _ = b.readRawByteArray()
		}
	}
	return out
}

func matchTemplate(obj *fakeP11Object, tmpl []p11Attribute) bool {
	for _, a := range tmpl {
		switch a.typ {
		case ckaClass:
			if obj.class != a.ulong {
				return false
			}
		case ckaCertificateType:
			if obj.certType != a.ulong {
				return false
			}
		case ckaID:
			if !bytes.Equal(obj.id, a.bytes) {
				return false
			}
		}
	}
	return true
}

func readMechanism(b *rpcBuf) p11Mechanism {
	typ, _ := b.readUint32()
	m := p11Mechanism{typ: typ}
	if typ == ckmRSAPKCSPSS {
		hasParam, _ := b.readByte()
		if hasParam != 0 {
			h, _ := b.readUint64()
			mgf, _ := b.readUint64()
			sLen, _ := b.readUint64()
			m.pssParams = &p11PSSParams{hashAlg: h, mgf: mgf, saltLen: sLen}
		}
	}
	return m
}

func fakeSign(signer crypto.Signer, mech p11Mechanism, data []byte) ([]byte, error) {
	switch mech.typ {
	case ckmECDSA:
		ecKey := signer.(*ecdsa.PrivateKey)
		r, s, err := ecdsa.Sign(rand.Reader, ecKey, data)
		if err != nil {
			return nil, err
		}
		byteLen := (ecKey.Curve.Params().BitSize + 7) / 8
		raw := make([]byte, 2*byteLen)
		r.FillBytes(raw[:byteLen])
		s.FillBytes(raw[byteLen:])
		return raw, nil

	case ckmRSAPKCS:
		rsaKey := signer.(*rsa.PrivateKey)
		for h, prefix := range rsaDigestInfoPrefixes {
			if bytes.HasPrefix(data, prefix) {
				digest := data[len(prefix):]
				return rsa.SignPKCS1v15(rand.Reader, rsaKey, h, digest)
			}
		}
		return nil, io.ErrUnexpectedEOF

	case ckmRSAPKCSPSS:
		rsaKey := signer.(*rsa.PrivateKey)
		var h crypto.Hash
		switch mech.pssParams.hashAlg {
		case ckmSHA1:
			h = crypto.SHA1
		case ckmSHA256:
			h = crypto.SHA256
		case ckmSHA384:
			h = crypto.SHA384
		case ckmSHA512:
			h = crypto.SHA512
		default:
			return nil, io.ErrUnexpectedEOF
		}
		return rsa.SignPSS(rand.Reader, rsaKey, h, data, &rsa.PSSOptions{
			SaltLength: int(mech.pssParams.saltLen),
			Hash:       h,
		})
	default:
		return nil, io.ErrUnexpectedEOF
	}
}

func generateTestCA(t *testing.T) (*x509.Certificate, crypto.Signer, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Nitro CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	return parsed, key, der
}

func generateTestLeaf(
	t *testing.T,
	caCert *x509.Certificate,
	caKey crypto.Signer,
	key crypto.Signer,
	dnsNames []string,
	notBefore time.Time,
	serial int64,
) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, key.Public(), caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}
	return der
}
