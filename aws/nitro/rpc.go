//go:build awsnitro

package nitro

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	p11RPCHandshake = "PRIVATE-GNOME-KEYRING-PKCS11-PROTOCOL-V-1"
	p11MaxFrameSize = 16 << 20
)

const (
	rpcCallError             uint32 = 0
	rpcCallCInitialize       uint32 = 1
	rpcCallCFinalize         uint32 = 2
	rpcCallCGetSlotList      uint32 = 4
	rpcCallCGetTokenInfo     uint32 = 6
	rpcCallCOpenSession      uint32 = 10
	rpcCallCCloseSession     uint32 = 11
	rpcCallCGetAttributeVal  uint32 = 24
	rpcCallCFindObjectsInit  uint32 = 26
	rpcCallCFindObjects      uint32 = 27
	rpcCallCFindObjectsFinal uint32 = 28
	rpcCallCSignInit         uint32 = 42
	rpcCallCSign             uint32 = 43
)

type rpcBuf struct {
	data []byte
	off  int
}

func newRPCBuf(capHint int) *rpcBuf {
	return &rpcBuf{data: make([]byte, 0, capHint)}
}

func (b *rpcBuf) writeByte(v byte) {
	b.data = append(b.data, v)
}

func (b *rpcBuf) writeUint32(v uint32) {
	b.data = binary.BigEndian.AppendUint32(b.data, v)
}

func (b *rpcBuf) writeUint64(v uint64) {
	b.data = binary.BigEndian.AppendUint64(b.data, v)
}

func (b *rpcBuf) writeRawByteArray(p []byte) {
	if p == nil {
		b.writeUint32(0xffffffff)
		return
	}
	b.writeUint32(uint32(len(p)))
	b.data = append(b.data, p...)
}

func (b *rpcBuf) writeByteArray(p []byte) {
	b.writeByte(1)
	b.writeRawByteArray(p)
}

func (b *rpcBuf) writeHeader(callID uint32, sig string) {
	b.writeUint32(callID)
	b.writeUint32(uint32(len(sig)))
	b.data = append(b.data, sig...)
}

func (b *rpcBuf) readByte() (byte, error) {
	if b.off+1 > len(b.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := b.data[b.off]
	b.off++
	return v, nil
}

func (b *rpcBuf) readUint32() (uint32, error) {
	if b.off+4 > len(b.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint32(b.data[b.off : b.off+4])
	b.off += 4
	return v, nil
}

func (b *rpcBuf) readUint64() (uint64, error) {
	if b.off+8 > len(b.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint64(b.data[b.off : b.off+8])
	b.off += 8
	return v, nil
}

func (b *rpcBuf) readRawByteArray() ([]byte, error) {
	n, err := b.readUint32()
	if err != nil {
		return nil, err
	}
	if n == 0xffffffff {
		return nil, nil
	}
	if n >= 0x7fffffff || b.off+int(n) > len(b.data) {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, n)
	copy(out, b.data[b.off:b.off+int(n)])
	b.off += int(n)
	return out, nil
}

func (b *rpcBuf) readByteArray() (data []byte, valid bool, needed uint32, err error) {
	v, err := b.readByte()
	if err != nil {
		return nil, false, 0, err
	}
	if v == 0 {
		n, err := b.readUint32()
		if err != nil {
			return nil, false, 0, err
		}
		return nil, false, n, nil
	}
	p, err := b.readRawByteArray()
	if err != nil {
		return nil, false, 0, err
	}
	return p, true, uint32(len(p)), nil
}

func (b *rpcBuf) readUlongArray() (vals []uint64, valid bool, needed uint32, err error) {
	v, err := b.readByte()
	if err != nil {
		return nil, false, 0, err
	}
	n, err := b.readUint32()
	if err != nil {
		return nil, false, 0, err
	}
	if v == 0 {
		return nil, false, n, nil
	}
	if n > uint32((len(b.data)-b.off)/8) {
		return nil, false, 0, io.ErrUnexpectedEOF
	}
	out := make([]uint64, n)
	for i := range out {
		u, err := b.readUint64()
		if err != nil {
			return nil, false, 0, err
		}
		out[i] = u
	}
	return out, true, n, nil
}

func (b *rpcBuf) readSpaceString(expectedLen int) (string, error) {
	p, err := b.readRawByteArray()
	if err != nil {
		return "", err
	}
	if len(p) != expectedLen {
		return "", fmt.Errorf("aws/nitro: invalid space-padded string length %d (want %d)", len(p), expectedLen)
	}
	return string(p), nil
}

type rpcConn struct {
	conn     net.Conn
	timeout  time.Duration
	nextCode uint32
}

func newRPCConn(conn net.Conn, timeout time.Duration) (*rpcConn, error) {
	rc := &rpcConn{
		conn:     conn,
		timeout:  timeout,
		nextCode: 0x10,
	}
	if err := rc.authenticate(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return rc, nil
}

func (rc *rpcConn) authenticate() error {
	if rc.timeout > 0 {
		_ = rc.conn.SetDeadline(time.Now().Add(rc.timeout))
		defer func() { _ = rc.conn.SetDeadline(time.Time{}) }()
	}

	// p11-kit RPC transport authentication exchanges a single protocol version byte.
	// Protocol version 0 is universally supported across p11-kit-server versions.
	var ver [1]byte
	if _, err := rc.conn.Write(ver[:]); err != nil {
		return fmt.Errorf("aws/nitro: rpc auth write: %w", err)
	}
	if _, err := io.ReadFull(rc.conn, ver[:]); err != nil {
		return fmt.Errorf("aws/nitro: rpc auth read: %w", err)
	}
	return nil
}

func (rc *rpcConn) close() error {
	if rc.conn == nil {
		return nil
	}
	return rc.conn.Close()
}

func (rc *rpcConn) roundTrip(callID uint32, req *rpcBuf, wantRespSig string) (*rpcBuf, error) {
	if rc.timeout > 0 {
		_ = rc.conn.SetDeadline(time.Now().Add(rc.timeout))
		defer func() { _ = rc.conn.SetDeadline(time.Time{}) }()
	}

	code := rc.nextCode
	rc.nextCode++
	if rc.nextCode == 0 {
		rc.nextCode = 0x10
	}

	var hdr [12]byte
	binary.BigEndian.PutUint32(hdr[0:4], code)
	binary.BigEndian.PutUint32(hdr[4:8], 0)
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(req.data)))

	if err := writeAll(rc.conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc write header: %w", err)
	}
	if err := writeAll(rc.conn, req.data); err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc write payload: %w", err)
	}

	if _, err := io.ReadFull(rc.conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc read header: %w", err)
	}
	respCode := binary.BigEndian.Uint32(hdr[0:4])
	optLen := binary.BigEndian.Uint32(hdr[4:8])
	bufLen := binary.BigEndian.Uint32(hdr[8:12])

	if respCode != code {
		return nil, fmt.Errorf("aws/nitro: rpc call code mismatch: got %d, want %d", respCode, code)
	}
	if optLen > p11MaxFrameSize || bufLen > p11MaxFrameSize {
		return nil, fmt.Errorf("aws/nitro: rpc frame too large: opt=%d buf=%d", optLen, bufLen)
	}
	if optLen > 0 {
		if _, err := io.CopyN(io.Discard, rc.conn, int64(optLen)); err != nil {
			return nil, fmt.Errorf("aws/nitro: rpc read options: %w", err)
		}
	}

	payload := make([]byte, bufLen)
	if _, err := io.ReadFull(rc.conn, payload); err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc read payload: %w", err)
	}

	resp := &rpcBuf{data: payload}
	respCallID, err := resp.readUint32()
	if err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc parse call id: %w", err)
	}
	sigBytes, err := resp.readRawByteArray()
	if err != nil {
		return nil, fmt.Errorf("aws/nitro: rpc parse signature: %w", err)
	}
	sig := string(sigBytes)

	if respCallID == rpcCallError {
		if sig != "u" {
			return nil, fmt.Errorf("aws/nitro: unexpected rpc error signature %q", sig)
		}
		rv, err := resp.readUint64()
		if err != nil {
			return nil, fmt.Errorf("aws/nitro: rpc parse error code: %w", err)
		}
		return nil, CKError(rv)
	}
	if respCallID != callID {
		return nil, fmt.Errorf("aws/nitro: rpc call id mismatch: got %d, want %d", respCallID, callID)
	}
	if sig != wantRespSig {
		return nil, fmt.Errorf("aws/nitro: rpc response signature mismatch: got %q, want %q", sig, wantRespSig)
	}
	return resp, nil
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("short write")
		}
		p = p[n:]
	}
	return nil
}
