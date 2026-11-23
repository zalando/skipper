//go:build awsnitro

package nitro

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
)

type (
	fakeKey struct {
		label  string
		signer crypto.Signer
	}

	fakeToken struct {
		label string
		pin   string
		keys  []fakeKey
	}

	fakeSession struct {
		slot     uint64
		loggedIn bool
		found    []uint64
		mech     *mechanism
		key      uint64
	}

	fakeServer struct {
		mu      sync.Mutex
		tokens  []*fakeToken
		version byte
		conns   map[net.Conn]struct{}
		calls   map[uint32]int
		wg      sync.WaitGroup
	}
)

func newFakeServer(tokens ...*fakeToken) *fakeServer {
	return &fakeServer{
		tokens: tokens,
		conns:  make(map[net.Conn]struct{}),
		calls:  make(map[uint32]int),
	}
}

func (f *fakeServer) dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	f.mu.Lock()
	f.conns[server] = struct{}{}
	f.mu.Unlock()

	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		defer func() {
			f.mu.Lock()
			delete(f.conns, server)
			f.mu.Unlock()
			server.Close()
		}()
		f.serve(server)
	}()
	return client, nil
}

func (f *fakeServer) setToken(i int, t *fakeToken) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[i] = t
}

// disconnect closes all connections, like an enclave restart.
func (f *fakeServer) disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.conns {
		c.Close()
	}
}

func (f *fakeServer) close() {
	f.disconnect()
	f.wg.Wait()
}

func (f *fakeServer) count(call uint32) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[call]
}

func (f *fakeServer) serve(conn net.Conn) {
	var v [1]byte
	if _, err := io.ReadFull(conn, v[:]); err != nil {
		return
	}
	if _, err := conn.Write([]byte{f.version}); err != nil {
		return
	}

	sessions := make(map[uint64]*fakeSession)
	var nextSession uint64 = 1
	for {
		code, body, err := readFrame(conn)
		if err != nil {
			return
		}
		resp := f.handle(body, sessions, &nextSession)
		if err := writeFrame(conn, code, resp); err != nil {
			return
		}
	}
}

func errorResponse(rv uint64) []byte {
	e := &encoder{}
	e.uint32(callError)
	e.rawByteArray([]byte(callSignatures[callError][1]))
	e.ulong(rv)
	return e.b
}

func (f *fakeServer) token(slot uint64) *fakeToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	if slot == 0 || slot > uint64(len(f.tokens)) {
		return nil
	}
	return f.tokens[slot-1]
}

func (f *fakeServer) handle(body []byte, sessions map[uint64]*fakeSession, nextSession *uint64) []byte {
	d := &decoder{b: body}
	call := d.uint32()
	sig := d.rawByteArray()
	sigs, ok := callSignatures[call]
	if d.err != nil || !ok || call == callError || string(sig) != sigs[0] {
		return errorResponse(ckrDeviceError)
	}

	f.mu.Lock()
	f.calls[call]++
	f.mu.Unlock()

	resp := &encoder{}
	resp.uint32(call)
	resp.rawByteArray([]byte(sigs[1]))

	session := func() *fakeSession { return sessions[d.ulong()] }

	switch call {
	case callInitialize:
		handshake, _ := d.byteArray()
		d.byte()
		d.byteArray()
		if string(handshake) != rpcHandshake {
			return errorResponse(ckrGeneralError)
		}
	case callFinalize:
	case callGetSlotList:
		d.byte()
		capacity := d.uint32()
		f.mu.Lock()
		n := uint32(len(f.tokens))
		f.mu.Unlock()
		if capacity < n {
			resp.byte(0)
			resp.uint32(n)
			break
		}
		resp.byte(1)
		resp.uint32(n)
		for i := range n {
			resp.ulong(uint64(i) + 1)
		}
	case callGetTokenInfo:
		t := f.token(d.ulong())
		if t == nil {
			return errorResponse(ckrArgumentsBad)
		}
		var flags uint64
		if t.pin != "" {
			flags = ckfLoginRequired
		}
		resp.rawByteArray(padded(t.label, 32))
		resp.rawByteArray(padded("Amazon", 32))
		resp.rawByteArray(padded("fake", 16))
		resp.rawByteArray(padded("1", 16))
		resp.ulong(flags)
		for range 10 {
			resp.ulong(0)
		}
		resp.b = append(resp.b, 1, 0, 1, 0)
		resp.rawByteArray(padded("", 16))
	case callOpenSession:
		slot := d.ulong()
		if d.ulong()&ckfSerialSession == 0 {
			return errorResponse(ckrArgumentsBad)
		}
		if f.token(slot) == nil {
			return errorResponse(ckrArgumentsBad)
		}
		h := *nextSession
		*nextSession++
		sessions[h] = &fakeSession{slot: slot}
		resp.ulong(h)
	case callCloseSession:
		h := d.ulong()
		if sessions[h] == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		delete(sessions, h)
	case callLogin:
		s := session()
		d.ulong()
		pin, _ := d.byteArray()
		if s == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		if string(pin) != f.token(s.slot).pin {
			return errorResponse(ckrPinIncorrect)
		}
		s.loggedIn = true
	case callFindObjectsInit:
		s := session()
		attrs := d.attributes()
		if s == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		t := f.token(s.slot)
		if t.pin != "" && !s.loggedIn {
			return errorResponse(ckrUserNotLoggedIn)
		}
		s.found = nil
		for i, k := range t.keys {
			if matches(attrs, k) {
				s.found = append(s.found, s.slot*100+uint64(i)+1)
			}
		}
	case callFindObjects:
		s := session()
		d.uint32()
		if s == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		resp.byte(1)
		resp.uint32(uint32(len(s.found)))
		for _, o := range s.found {
			resp.ulong(o)
		}
	case callFindObjectsFinal:
		if session() == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
	case callSignInit:
		s := session()
		m := d.mechanism()
		key := d.ulong()
		if s == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		if f.key(s.slot, key) == nil {
			return errorResponse(ckrKeyHandleInvalid)
		}
		s.mech, s.key = &m, key
	case callSign:
		s := session()
		data, _ := d.byteArray()
		capacity := d.uint32()
		if s == nil {
			return errorResponse(ckrSessionHandleInvalid)
		}
		if s.mech == nil {
			return errorResponse(ckrOperationNotInitialized)
		}
		key := f.key(s.slot, s.key)
		m := s.mech
		s.mech = nil
		if key == nil {
			return errorResponse(ckrKeyHandleInvalid)
		}
		sig, err := fakeSign(key, *m, data)
		if err != nil {
			return errorResponse(ckrMechanismInvalid)
		}
		if uint32(len(sig)) > capacity {
			resp.byte(0)
			resp.uint32(uint32(len(sig)))
			break
		}
		resp.byteArray(sig)
	}

	if err := d.done(); err != nil {
		return errorResponse(ckrArgumentsBad)
	}
	return resp.b
}

func (f *fakeServer) key(slot, handle uint64) crypto.Signer {
	t := f.token(slot)
	if t == nil || handle/100 != slot {
		return nil
	}
	i := handle%100 - 1
	if i >= uint64(len(t.keys)) {
		return nil
	}
	return t.keys[i].signer
}

func matches(attrs []attribute, k fakeKey) bool {
	for _, a := range attrs {
		switch a.typ {
		case ckaClass:
			if a.num != ckoPrivateKey {
				return false
			}
		case ckaLabel:
			if string(a.value) != k.label {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func fakeSign(key crypto.Signer, m mechanism, data []byte) ([]byte, error) {
	switch m.typ {
	case ckmRSAPKCS:
		k, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an rsa key")
		}
		return rsa.SignPKCS1v15(rand.Reader, k, 0, data)
	case ckmRSAPKCSPSS:
		k, ok := key.(*rsa.PrivateKey)
		if !ok || m.pss == nil {
			return nil, errors.New("invalid pss")
		}
		hashes := map[uint64]crypto.Hash{ckmSHA1: crypto.SHA1, ckmSHA224: crypto.SHA224, ckmSHA256: crypto.SHA256, ckmSHA384: crypto.SHA384, ckmSHA512: crypto.SHA512}
		h, ok := hashes[m.pss.hashAlg]
		if !ok || pssHash[h][1] != m.pss.mgf {
			return nil, errors.New("invalid pss hash")
		}
		return rsa.SignPSS(rand.Reader, k, h, data, &rsa.PSSOptions{SaltLength: int(m.pss.sLen), Hash: h})
	case ckmECDSA:
		k, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an ecdsa key")
		}
		r, s, err := ecdsa.Sign(rand.Reader, k, data)
		if err != nil {
			return nil, err
		}
		n := (k.Curve.Params().BitSize + 7) / 8
		return append(fixed(r, n), fixed(s, n)...), nil
	default:
		return nil, errors.New("unsupported mechanism")
	}
}

func fixed(v *big.Int, n int) []byte {
	b := make([]byte, n)
	return v.FillBytes(b)
}

func padded(s string, n int) []byte {
	b := []byte(s)
	for len(b) < n {
		b = append(b, ' ')
	}
	return b
}
