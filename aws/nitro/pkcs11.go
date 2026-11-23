//go:build awsnitro

package nitro

import (
	"fmt"
	"strings"
)

const (
	ckrOK                   uint64 = 0x00000000
	ckrAttributeTypeInvalid uint64 = 0x00000012
	ckrBufferTooSmall       uint64 = 0x00000150
	ckrSessionClosed        uint64 = 0x000000b0
	ckrSessionHandleInvalid uint64 = 0x000000b3
)

const (
	ckfSerialSession uint64 = 0x00000004
)

const (
	ckoCertificate uint64 = 0x00000001
	ckoPrivateKey  uint64 = 0x00000003
)

const (
	ckcX509 uint64 = 0x00000000
)

const (
	ckaClass           uint32 = 0x00000000
	ckaToken           uint32 = 0x00000001
	ckaValue           uint32 = 0x00000011
	ckaCertificateType uint32 = 0x00000080
	ckaKeyType         uint32 = 0x00000100
	ckaID              uint32 = 0x00000102
	ckaSign            uint32 = 0x00000108
)

const (
	ckmRSAPKCS    uint32 = 0x00000001
	ckmRSAPKCSPSS uint32 = 0x0000000d
	ckmSHA1       uint64 = 0x00000220
	ckmSHA256     uint64 = 0x00000250
	ckmSHA384     uint64 = 0x00000260
	ckmSHA512     uint64 = 0x00000270
	ckmECDSA      uint32 = 0x00001041
)

const (
	ckgMGF1SHA1   uint64 = 0x00000001
	ckgMGF1SHA256 uint64 = 0x00000002
	ckgMGF1SHA384 uint64 = 0x00000003
	ckgMGF1SHA512 uint64 = 0x00000004
)

// CKError represents a non-zero PKCS#11 CK_RV return code from the enclave.
type CKError uint64

func (e CKError) Error() string {
	return fmt.Sprintf("aws/nitro: pkcs11 error 0x%08x", uint64(e))
}

type p11ValueKind int

const (
	p11ValByteArray p11ValueKind = iota
	p11ValByte
	p11ValUlong
)

func attrValueKind(attrType uint32) p11ValueKind {
	switch attrType {
	case ckaToken, ckaSign:
		return p11ValByte
	case ckaClass, ckaCertificateType, ckaKeyType:
		return p11ValUlong
	default:
		return p11ValByteArray
	}
}

type p11Attribute struct {
	typ     uint32
	valid   bool
	byteVal byte
	ulong   uint64
	bytes   []byte
}

func attrUlong(typ uint32, v uint64) p11Attribute {
	return p11Attribute{typ: typ, valid: true, ulong: v}
}

func attrBytes(typ uint32, v []byte) p11Attribute {
	return p11Attribute{typ: typ, valid: true, bytes: v}
}

type p11AttrRequest struct {
	typ    uint32
	bufLen uint32
}

type p11PSSParams struct {
	hashAlg uint64
	mgf     uint64
	saltLen uint64
}

type p11Mechanism struct {
	typ       uint32
	pssParams *p11PSSParams
}

func writeAttributeArray(b *rpcBuf, attrs []p11Attribute) {
	b.writeUint32(uint32(len(attrs)))
	for _, a := range attrs {
		b.writeUint32(a.typ)
		if !a.valid {
			b.writeByte(0)
			continue
		}
		b.writeByte(1)
		switch attrValueKind(a.typ) {
		case p11ValByte:
			b.writeUint32(1)
			b.writeByte(a.byteVal)
		case p11ValUlong:
			b.writeUint32(8)
			b.writeUint64(a.ulong)
		default:
			b.writeUint32(uint32(len(a.bytes)))
			b.writeRawByteArray(a.bytes)
		}
	}
}

func writeAttributeBuffer(b *rpcBuf, reqs []p11AttrRequest) {
	b.writeUint32(uint32(len(reqs)))
	for _, r := range reqs {
		b.writeUint32(r.typ)
		b.writeUint32(r.bufLen)
	}
}

func readAttributeArray(b *rpcBuf, expected int) ([]p11Attribute, uint64, error) {
	n, err := b.readUint32()
	if err != nil {
		return nil, 0, err
	}
	if int(n) != expected {
		return nil, 0, fmt.Errorf("aws/nitro: attribute count mismatch: got %d, want %d", n, expected)
	}
	out := make([]p11Attribute, n)
	for i := range out {
		typ, err := b.readUint32()
		if err != nil {
			return nil, 0, err
		}
		valid, err := b.readByte()
		if err != nil {
			return nil, 0, err
		}
		out[i].typ = typ
		if valid == 0 {
			continue
		}
		out[i].valid = true
		if _, err := b.readUint32(); err != nil {
			return nil, 0, err
		}
		switch attrValueKind(typ) {
		case p11ValByte:
			v, err := b.readByte()
			if err != nil {
				return nil, 0, err
			}
			out[i].byteVal = v
		case p11ValUlong:
			v, err := b.readUint64()
			if err != nil {
				return nil, 0, err
			}
			out[i].ulong = v
		default:
			v, err := b.readRawByteArray()
			if err != nil {
				return nil, 0, err
			}
			out[i].bytes = v
		}
	}
	rv, err := b.readUint64()
	if err != nil {
		return nil, 0, err
	}
	return out, rv, nil
}

func writeMechanism(b *rpcBuf, m p11Mechanism) error {
	b.writeUint32(m.typ)
	switch m.typ {
	case ckmRSAPKCS, ckmECDSA:
		return nil
	case ckmRSAPKCSPSS:
		if m.pssParams == nil {
			b.writeByte(0)
			return nil
		}
		b.writeByte(1)
		b.writeUint64(m.pssParams.hashAlg)
		b.writeUint64(m.pssParams.mgf)
		b.writeUint64(m.pssParams.saltLen)
		return nil
	default:
		return fmt.Errorf("aws/nitro: unsupported mechanism 0x%08x", m.typ)
	}
}

type p11Client struct {
	rpc *rpcConn
}

func newP11Client(rc *rpcConn) (*p11Client, error) {
	c := &p11Client{rpc: rc}
	if err := c.initialize(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *p11Client) close() error {
	if c.rpc == nil {
		return nil
	}
	req := newRPCBuf(16)
	req.writeHeader(rpcCallCFinalize, "")
	_, _ = c.rpc.roundTrip(rpcCallCFinalize, req, "")
	return c.rpc.close()
}

func (c *p11Client) initialize() error {
	req := newRPCBuf(80)
	req.writeHeader(rpcCallCInitialize, "ayyay")
	req.writeByteArray([]byte(p11RPCHandshake))
	req.writeByte(0)
	req.writeByteArray([]byte{0})
	_, err := c.rpc.roundTrip(rpcCallCInitialize, req, "")
	return err
}

func (c *p11Client) getSlotList(tokenPresent bool) ([]uint64, error) {
	var tp byte
	if tokenPresent {
		tp = 1
	}

	maxSlots := uint32(64)
	for range 2 {
		req := newRPCBuf(32)
		req.writeHeader(rpcCallCGetSlotList, "yfu")
		req.writeByte(tp)
		req.writeUint32(maxSlots)

		resp, err := c.rpc.roundTrip(rpcCallCGetSlotList, req, "au")
		if err != nil {
			return nil, err
		}
		slots, valid, needed, err := resp.readUlongArray()
		if err != nil {
			return nil, err
		}
		if valid {
			return slots, nil
		}
		maxSlots = needed
	}
	return nil, CKError(ckrBufferTooSmall)
}

func (c *p11Client) getTokenLabel(slotID uint64) (string, error) {
	req := newRPCBuf(32)
	req.writeHeader(rpcCallCGetTokenInfo, "u")
	req.writeUint64(slotID)

	resp, err := c.rpc.roundTrip(rpcCallCGetTokenInfo, req, "ssssuuuuuuuuuuuvvs")
	if err != nil {
		return "", err
	}
	label, err := resp.readSpaceString(32)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(label, " \x00"), nil
}

func (c *p11Client) openSession(slotID uint64) (uint64, error) {
	req := newRPCBuf(32)
	req.writeHeader(rpcCallCOpenSession, "uu")
	req.writeUint64(slotID)
	req.writeUint64(ckfSerialSession)

	resp, err := c.rpc.roundTrip(rpcCallCOpenSession, req, "u")
	if err != nil {
		return 0, err
	}
	return resp.readUint64()
}

func (c *p11Client) closeSession(session uint64) error {
	req := newRPCBuf(24)
	req.writeHeader(rpcCallCCloseSession, "u")
	req.writeUint64(session)
	_, err := c.rpc.roundTrip(rpcCallCCloseSession, req, "")
	return err
}

func (c *p11Client) findObjects(session uint64, template []p11Attribute) ([]uint64, error) {
	req := newRPCBuf(64)
	req.writeHeader(rpcCallCFindObjectsInit, "uaA")
	req.writeUint64(session)
	writeAttributeArray(req, template)
	if _, err := c.rpc.roundTrip(rpcCallCFindObjectsInit, req, ""); err != nil {
		return nil, err
	}
	defer func() {
		fin := newRPCBuf(24)
		fin.writeHeader(rpcCallCFindObjectsFinal, "u")
		fin.writeUint64(session)
		_, _ = c.rpc.roundTrip(rpcCallCFindObjectsFinal, fin, "")
	}()

	var handles []uint64
	for {
		findReq := newRPCBuf(32)
		findReq.writeHeader(rpcCallCFindObjects, "ufu")
		findReq.writeUint64(session)
		findReq.writeUint32(64)

		resp, err := c.rpc.roundTrip(rpcCallCFindObjects, findReq, "au")
		if err != nil {
			return nil, err
		}
		batch, valid, _, err := resp.readUlongArray()
		if err != nil {
			return nil, err
		}
		if !valid || len(batch) == 0 {
			break
		}
		handles = append(handles, batch...)
	}
	return handles, nil
}

func (c *p11Client) getAttributes(session, obj uint64, reqs []p11AttrRequest) ([]p11Attribute, error) {
	req := newRPCBuf(64)
	req.writeHeader(rpcCallCGetAttributeVal, "uufA")
	req.writeUint64(session)
	req.writeUint64(obj)
	writeAttributeBuffer(req, reqs)

	resp, err := c.rpc.roundTrip(rpcCallCGetAttributeVal, req, "aAu")
	if err != nil {
		return nil, err
	}
	attrs, rv, err := readAttributeArray(resp, len(reqs))
	if err != nil {
		return nil, err
	}
	if rv != ckrOK && rv != ckrAttributeTypeInvalid {
		return attrs, CKError(rv)
	}
	return attrs, nil
}

func (c *p11Client) sign(session, key uint64, mech p11Mechanism, data []byte) ([]byte, error) {
	initReq := newRPCBuf(64)
	initReq.writeHeader(rpcCallCSignInit, "uMu")
	initReq.writeUint64(session)
	if err := writeMechanism(initReq, mech); err != nil {
		return nil, err
	}
	initReq.writeUint64(key)
	if _, err := c.rpc.roundTrip(rpcCallCSignInit, initReq, ""); err != nil {
		return nil, err
	}

	maxSigLen := uint32(1024)
	for range 2 {
		signReq := newRPCBuf(64 + len(data))
		signReq.writeHeader(rpcCallCSign, "uayfy")
		signReq.writeUint64(session)
		signReq.writeByteArray(data)
		signReq.writeUint32(maxSigLen)

		resp, err := c.rpc.roundTrip(rpcCallCSign, signReq, "ay")
		if err != nil {
			return nil, err
		}
		sig, valid, needed, err := resp.readByteArray()
		if err != nil {
			return nil, err
		}
		if valid {
			return sig, nil
		}
		maxSigLen = needed
	}
	return nil, CKError(ckrBufferTooSmall)
}
