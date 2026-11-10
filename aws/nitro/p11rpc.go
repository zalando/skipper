//go:build awsnitro

package nitro

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	p11RPCHandshake = "PRIVATE-GNOME-KEYRING-PKCS11-PROTOCOL-V-1"
	p11ModuleName   = "p11ne"

	p11CallError             = 0
	p11CallInitialize        = 1
	p11CallFinalize          = 2
	p11CallGetSlotList       = 4
	p11CallGetTokenInfo      = 6
	p11CallOpenSession       = 10
	p11CallCloseSession      = 11
	p11CallLogin             = 18
	p11CallGetAttributeValue = 24
	p11CallFindObjectsInit   = 26
	p11CallFindObjects       = 27
	p11CallFindObjectsFinal  = 28
	p11CallSignInit          = 42
	p11CallSign              = 43

	ckrOK                  = 0x00000000
	ckrUserAlreadyLoggedIn = 0x00000100

	ckfSerialSession = 0x00000004
	ckfLoginRequired = 0x00000004

	ckuUser = 1

	ckoCertificate = 0x00000001
	ckoPrivateKey  = 0x00000003

	ckaClass               = 0x00000000
	ckaToken               = 0x00000001
	ckaPrivate             = 0x00000002
	ckaValue               = 0x00000011
	ckaCertificateType     = 0x00000080
	ckaCertificateCategory = 0x00000087
	ckaKeyType             = 0x00000100
	ckaID                  = 0x00000102
	ckaSensitive           = 0x00000103
	ckaEncrypt             = 0x00000104
	ckaDecrypt             = 0x00000105
	ckaSign                = 0x00000108
	ckaVerify              = 0x0000010A
	ckaModulusBits         = 0x00000121

	ckCertCategoryTokenUser = 1

	ckmRSAPKCS    = 0x00000001
	ckmRSAPKCSPSS = 0x0000000D
	ckmSHA1       = 0x00000220
	ckmSHA224     = 0x00000255
	ckmSHA256     = 0x00000250
	ckmSHA384     = 0x00000260
	ckmSHA512     = 0x00000270
	ckmECDSA      = 0x00001041

	ckgMGF1SHA1   = 0x00000001
	ckgMGF1SHA256 = 0x00000002
	ckgMGF1SHA384 = 0x00000003
	ckgMGF1SHA512 = 0x00000004
	ckgMGF1SHA224 = 0x00000005
)

type (
	p11Error uint64

	p11TokenInfo struct {
		Label        string
		Manufacturer string
		Model        string
		SerialNumber string
		Flags        uint64
		MaxSessions  uint64
	}

	p11Attribute struct {
		Type  uint32
		Value []byte
		ULong uint64
		Byte  byte
		Valid bool
	}

	p11Mechanism struct {
		Type      uint32
		PSSParams *p11RSAPSSParams
	}

	p11RSAPSSParams struct {
		HashAlg uint64
		MGF     uint64
		SLen    uint64
	}

	p11RPCClient struct {
		mu       sync.Mutex
		conn     io.ReadWriteCloser
		callCode uint32
		version  byte
		closed   atomic.Bool
	}

	rpcBuf struct {
		b   []byte
		off int
	}
)

func (e p11Error) Error() string {
	return fmt.Sprintf("pkcs11 error: 0x%08x", uint64(e))
}

func newP11RPCClient(conn io.ReadWriteCloser) (*p11RPCClient, error) {
	c := &p11RPCClient{
		conn:     conn,
		callCode: 0x10,
		version:  2,
	}
	if _, err := conn.Write([]byte{c.version}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to write p11-kit rpc version: %w", err)
	}
	var ver [1]byte
	if _, err := io.ReadFull(conn, ver[:]); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read p11-kit rpc version: %w", err)
	}
	c.version = ver[0]

	if err := c.initialize(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *p11RPCClient) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	_ = c.finalize()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

func (c *p11RPCClient) call(callID uint32, sig string, build func(*rpcBuf)) (*rpcBuf, error) {
	var req rpcBuf
	req.writeUint32(callID)
	req.writeBytes([]byte(sig))
	if build != nil {
		build(&req)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return nil, errors.New("p11-kit rpc client closed")
	}

	c.callCode++
	code := c.callCode
	opts := []byte(p11ModuleName)

	var hdr [12]byte
	binary.BigEndian.PutUint32(hdr[0:4], code)
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(opts)))
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(req.b)))

	if _, err := c.conn.Write(hdr[:]); err != nil {
		return nil, fmt.Errorf("rpc write header: %w", err)
	}
	if _, err := c.conn.Write(opts); err != nil {
		return nil, fmt.Errorf("rpc write options: %w", err)
	}
	if _, err := c.conn.Write(req.b); err != nil {
		return nil, fmt.Errorf("rpc write payload: %w", err)
	}

	if _, err := io.ReadFull(c.conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("rpc read header: %w", err)
	}
	respCode := binary.BigEndian.Uint32(hdr[0:4])
	olen := binary.BigEndian.Uint32(hdr[4:8])
	dlen := binary.BigEndian.Uint32(hdr[8:12])
	if respCode != code {
		return nil, fmt.Errorf("rpc call code mismatch: got %d, want %d", respCode, code)
	}
	if olen > 1<<20 || dlen > 16<<20 {
		return nil, fmt.Errorf("rpc response too large: olen=%d dlen=%d", olen, dlen)
	}

	if olen > 0 {
		discard := make([]byte, olen)
		if _, err := io.ReadFull(c.conn, discard); err != nil {
			return nil, fmt.Errorf("rpc read options: %w", err)
		}
	}

	data := make([]byte, dlen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, fmt.Errorf("rpc read payload: %w", err)
	}

	resp := &rpcBuf{b: data}
	respCallID, err := resp.readUint32()
	if err != nil {
		return nil, fmt.Errorf("rpc parse call id: %w", err)
	}
	respSig, err := resp.readBytes()
	if err != nil {
		return nil, fmt.Errorf("rpc parse signature: %w", err)
	}

	if respCallID == p11CallError {
		if string(respSig) != "u" {
			return nil, fmt.Errorf("invalid error signature: %q", string(respSig))
		}
		ckerr, err := resp.readUint64()
		if err != nil {
			return nil, err
		}
		return nil, p11Error(ckerr)
	}

	if respCallID != callID {
		return nil, fmt.Errorf("rpc call id mismatch: got %d, want %d", respCallID, callID)
	}
	return resp, nil
}

func (c *p11RPCClient) initialize() error {
	_, err := c.call(p11CallInitialize, "ayyay", func(b *rpcBuf) {
		b.writeByteArray([]byte(p11RPCHandshake))
		b.writeByte(0)
		b.writeByteArray([]byte{0})
	})
	return err
}

func (c *p11RPCClient) finalize() error {
	_, err := c.call(p11CallFinalize, "", nil)
	return err
}

func (c *p11RPCClient) getSlotList(tokenPresent bool) ([]uint64, error) {
	present := byte(0)
	if tokenPresent {
		present = 1
	}
	resp, err := c.call(p11CallGetSlotList, "yfu", func(b *rpcBuf) {
		b.writeByte(present)
		b.writeUint32(0)
	})
	if err != nil {
		return nil, err
	}
	_, count, err := resp.readUlongArray()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	resp, err = c.call(p11CallGetSlotList, "yfu", func(b *rpcBuf) {
		b.writeByte(present)
		b.writeUint32(count)
	})
	if err != nil {
		return nil, err
	}
	slots, _, err := resp.readUlongArray()
	return slots, err
}

func (c *p11RPCClient) getTokenInfo(slotID uint64) (*p11TokenInfo, error) {
	resp, err := c.call(p11CallGetTokenInfo, "u", func(b *rpcBuf) {
		b.writeUint64(slotID)
	})
	if err != nil {
		return nil, err
	}
	label, err := resp.readSpaceString()
	if err != nil {
		return nil, err
	}
	manufacturer, err := resp.readSpaceString()
	if err != nil {
		return nil, err
	}
	model, err := resp.readSpaceString()
	if err != nil {
		return nil, err
	}
	serial, err := resp.readSpaceString()
	if err != nil {
		return nil, err
	}
	flags, err := resp.readUint64()
	if err != nil {
		return nil, err
	}
	maxSessions, err := resp.readUint64()
	if err != nil {
		return nil, err
	}
	return &p11TokenInfo{
		Label:        label,
		Manufacturer: manufacturer,
		Model:        model,
		SerialNumber: serial,
		Flags:        flags,
		MaxSessions:  maxSessions,
	}, nil
}

func (c *p11RPCClient) openSession(slotID uint64) (uint64, error) {
	resp, err := c.call(p11CallOpenSession, "uu", func(b *rpcBuf) {
		b.writeUint64(slotID)
		b.writeUint64(ckfSerialSession)
	})
	if err != nil {
		return 0, err
	}
	return resp.readUint64()
}

func (c *p11RPCClient) closeSession(session uint64) error {
	_, err := c.call(p11CallCloseSession, "u", func(b *rpcBuf) {
		b.writeUint64(session)
	})
	return err
}

func (c *p11RPCClient) login(session uint64, pin string) error {
	_, err := c.call(p11CallLogin, "uuay", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeUint64(ckuUser)
		b.writeByteArray([]byte(pin))
	})
	var pErr p11Error
	if errors.As(err, &pErr) && pErr == ckrUserAlreadyLoggedIn {
		return nil
	}
	return err
}

func (c *p11RPCClient) findObjects(session uint64, attrs []p11Attribute) ([]uint64, error) {
	_, err := c.call(p11CallFindObjectsInit, "uaA", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeAttributeArray(attrs)
	})
	if err != nil {
		return nil, err
	}

	var all []uint64
	for {
		resp, err := c.call(p11CallFindObjects, "ufu", func(b *rpcBuf) {
			b.writeUint64(session)
			b.writeUint32(64)
		})
		if err != nil {
			_, _ = c.call(p11CallFindObjectsFinal, "u", func(b *rpcBuf) { b.writeUint64(session) })
			return nil, err
		}
		objs, _, err := resp.readUlongArray()
		if err != nil {
			_, _ = c.call(p11CallFindObjectsFinal, "u", func(b *rpcBuf) { b.writeUint64(session) })
			return nil, err
		}
		if len(objs) == 0 {
			break
		}
		all = append(all, objs...)
	}

	if _, err := c.call(p11CallFindObjectsFinal, "u", func(b *rpcBuf) {
		b.writeUint64(session)
	}); err != nil {
		return nil, err
	}
	return all, nil
}

func (c *p11RPCClient) getAttributeValue(session, object uint64, attrTypes []uint32) ([]p11Attribute, error) {
	resp, err := c.call(p11CallGetAttributeValue, "uufA", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeUint64(object)
		b.writeUint32(uint32(len(attrTypes)))
		for _, t := range attrTypes {
			b.writeUint32(t)
			b.writeUint32(0)
		}
	})
	if err != nil {
		return nil, err
	}
	sizes, retCode, err := resp.readAttributeArray()
	if err != nil {
		return nil, err
	}
	if retCode != ckrOK {
		return nil, p11Error(retCode)
	}

	resp, err = c.call(p11CallGetAttributeValue, "uufA", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeUint64(object)
		b.writeUint32(uint32(len(sizes)))
		for _, a := range sizes {
			b.writeUint32(a.Type)
			if a.Valid {
				b.writeUint32(uint32(len(a.Value)))
			} else {
				b.writeUint32(0)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	attrs, retCode, err := resp.readAttributeArray()
	if err != nil {
		return nil, err
	}
	if retCode != ckrOK {
		return nil, p11Error(retCode)
	}
	return attrs, nil
}

func (c *p11RPCClient) sign(session, keyHandle uint64, mech p11Mechanism, data []byte, maxSigLen uint32) ([]byte, error) {
	_, err := c.call(p11CallSignInit, "uMu", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeMechanism(mech)
		b.writeUint64(keyHandle)
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.call(p11CallSign, "uayfy", func(b *rpcBuf) {
		b.writeUint64(session)
		b.writeByteArray(data)
		b.writeUint32(maxSigLen)
	})
	if err != nil {
		return nil, err
	}
	sig, _, err := resp.readByteArray()
	return sig, err
}

func attrValueType(attrType uint32) int {
	switch attrType {
	case ckaToken, ckaPrivate, ckaSensitive, ckaEncrypt, ckaDecrypt, ckaSign, ckaVerify:
		return 0
	case ckaClass, ckaCertificateType, ckaCertificateCategory, ckaKeyType, ckaModulusBits:
		return 1
	default:
		return 5
	}
}

func (b *rpcBuf) writeByte(v byte) {
	b.b = append(b.b, v)
}

func (b *rpcBuf) writeUint32(v uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], v)
	b.b = append(b.b, tmp[:]...)
}

func (b *rpcBuf) writeUint64(v uint64) {
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], v)
	b.b = append(b.b, tmp[:]...)
}

func (b *rpcBuf) writeBytes(data []byte) {
	if data == nil {
		b.writeUint32(0xffffffff)
		return
	}
	b.writeUint32(uint32(len(data)))
	b.b = append(b.b, data...)
}

func (b *rpcBuf) writeByteArray(data []byte) {
	b.writeByte(1)
	b.writeUint32(uint32(len(data)))
	b.b = append(b.b, data...)
}

func (b *rpcBuf) writeAttributeArray(attrs []p11Attribute) {
	b.writeUint32(uint32(len(attrs)))
	for _, a := range attrs {
		b.writeUint32(a.Type)
		if !a.Valid {
			b.writeByte(0)
			continue
		}
		b.writeByte(1)
		switch attrValueType(a.Type) {
		case 0:
			b.writeUint32(1)
			b.writeByte(a.Byte)
		case 1:
			b.writeUint32(8)
			b.writeUint64(a.ULong)
		default:
			b.writeUint32(uint32(len(a.Value)))
			b.writeBytes(a.Value)
		}
	}
}

func (b *rpcBuf) writeMechanism(m p11Mechanism) {
	b.writeUint32(m.Type)
	if m.Type == ckmRSAPKCS || m.Type == ckmECDSA {
		return
	}
	if m.PSSParams == nil {
		b.writeByte(0)
		return
	}
	b.writeByte(1)
	b.writeUint64(m.PSSParams.HashAlg)
	b.writeUint64(m.PSSParams.MGF)
	b.writeUint64(m.PSSParams.SLen)
}

func (b *rpcBuf) readByte() (byte, error) {
	if b.off+1 > len(b.b) {
		return 0, io.ErrUnexpectedEOF
	}
	v := b.b[b.off]
	b.off++
	return v, nil
}

func (b *rpcBuf) readUint32() (uint32, error) {
	if b.off+4 > len(b.b) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint32(b.b[b.off : b.off+4])
	b.off += 4
	return v, nil
}

func (b *rpcBuf) readUint64() (uint64, error) {
	if b.off+8 > len(b.b) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint64(b.b[b.off : b.off+8])
	b.off += 8
	return v, nil
}

func (b *rpcBuf) readBytes() ([]byte, error) {
	n, err := b.readUint32()
	if err != nil {
		return nil, err
	}
	if n == 0xffffffff {
		return nil, nil
	}
	if n >= 0x7fffffff || b.off+int(n) > len(b.b) {
		return nil, io.ErrUnexpectedEOF
	}
	out := bytes.Clone(b.b[b.off : b.off+int(n)])
	b.off += int(n)
	return out, nil
}

func (b *rpcBuf) readByteArray() ([]byte, uint32, error) {
	valid, err := b.readByte()
	if err != nil {
		return nil, 0, err
	}
	if valid == 0 {
		n, err := b.readUint32()
		return nil, n, err
	}
	data, err := b.readBytes()
	if err != nil {
		return nil, 0, err
	}
	return data, uint32(len(data)), nil
}

func (b *rpcBuf) readUlongArray() ([]uint64, uint32, error) {
	valid, err := b.readByte()
	if err != nil {
		return nil, 0, err
	}
	num, err := b.readUint32()
	if err != nil {
		return nil, 0, err
	}
	if valid == 0 {
		return nil, num, nil
	}
	if num > 1<<20 {
		return nil, 0, errors.New("ulong array too large")
	}
	out := make([]uint64, num)
	for i := range num {
		v, err := b.readUint64()
		if err != nil {
			return nil, 0, err
		}
		out[i] = v
	}
	return out, num, nil
}

func (b *rpcBuf) readSpaceString() (string, error) {
	data, err := b.readBytes()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), " \x00"), nil
}

func (b *rpcBuf) readAttributeArray() ([]p11Attribute, uint64, error) {
	num, err := b.readUint32()
	if err != nil {
		return nil, 0, err
	}
	if num > 1024 {
		return nil, 0, errors.New("attribute array too large")
	}
	attrs := make([]p11Attribute, num)
	for i := range num {
		attrType, err := b.readUint32()
		if err != nil {
			return nil, 0, err
		}
		validity, err := b.readByte()
		if err != nil {
			return nil, 0, err
		}
		if validity == 0 {
			attrs[i] = p11Attribute{Type: attrType, Valid: false}
			continue
		}
		length, err := b.readUint32()
		if err != nil {
			return nil, 0, err
		}
		a := p11Attribute{Type: attrType, Valid: true}
		switch attrValueType(attrType) {
		case 0:
			v, err := b.readByte()
			if err != nil {
				return nil, 0, err
			}
			a.Byte = v
			a.Value = []byte{v}
		case 1:
			v, err := b.readUint64()
			if err != nil {
				return nil, 0, err
			}
			a.ULong = v
			a.Value = make([]byte, 8)
			binary.LittleEndian.PutUint64(a.Value, v)
		default:
			val, err := b.readBytes()
			if err != nil {
				return nil, 0, err
			}
			if val == nil && length > 0 {
				a.Value = make([]byte, length)
			} else {
				a.Value = val
			}
		}
		attrs[i] = a
	}
	retCode, err := b.readUint64()
	if err != nil {
		return nil, 0, err
	}
	return attrs, retCode, nil
}
