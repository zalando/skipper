//go:build awsnitro

package nitro

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type rpcConn struct {
	mu      sync.Mutex
	conn    net.Conn
	code    uint32
	timeout time.Duration
}

func newRPCConn(ctx context.Context, dial func(context.Context) (net.Conn, error), timeout time.Duration) (*rpcConn, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	c := &rpcConn{conn: conn, timeout: timeout}
	if err := c.authenticate(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// authenticate negotiates the protocol version, see
// rpc_transport_authenticate in p11-kit/rpc-transport.c.
func (c *rpcConn) authenticate(ctx context.Context) error {
	c.setDeadline(ctx)
	if _, err := c.conn.Write([]byte{rpcProtocolVersion}); err != nil {
		return fmt.Errorf("failed to send p11-kit rpc version: %w", err)
	}
	var v [1]byte
	if _, err := io.ReadFull(c.conn, v[:]); err != nil {
		return fmt.Errorf("failed to receive p11-kit rpc version: %w", err)
	}
	if v[0] != rpcProtocolVersion {
		return fmt.Errorf("%w: unsupported protocol version %d", errProtocol, v[0])
	}
	return nil
}

func (c *rpcConn) setDeadline(ctx context.Context) {
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.conn.SetDeadline(deadline)
}

func (c *rpcConn) Close() error {
	return c.conn.Close()
}

// call sends the request and returns a decoder positioned after the
// response signature. A *ckError is returned if the server returned a
// PKCS#11 error, other errors leave the connection in an undefined
// state and the connection must be closed.
func (c *rpcConn) call(ctx context.Context, req *encoder) (*decoder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.setDeadline(ctx)

	c.code++
	if c.code == 0 {
		c.code = 1
	}

	if err := writeFrame(c.conn, c.code, req.b); err != nil {
		return nil, err
	}
	code, body, err := readFrame(c.conn)
	if err != nil {
		return nil, err
	}
	if code != c.code {
		return nil, fmt.Errorf("%w: unexpected response code %d, expected %d", errProtocol, code, c.code)
	}
	return parseResponse(binary.BigEndian.Uint32(req.b), body)
}

func parseResponse(call uint32, body []byte) (*decoder, error) {
	d := &decoder{b: body}
	respCall := d.uint32()
	sig := d.rawByteArray()
	if d.err != nil {
		return nil, d.err
	}

	if respCall == callError {
		if string(sig) != callSignatures[callError][1] {
			return nil, fmt.Errorf("%w: invalid error signature %q", errProtocol, sig)
		}
		rv := d.ulong()
		if err := d.done(); err != nil {
			return nil, err
		}
		if rv == ckrOK {
			return nil, fmt.Errorf("%w: error response without error code", errProtocol)
		}
		return nil, ckError(rv)
	}

	if respCall != call {
		return nil, fmt.Errorf("%w: unexpected call %d in response, expected %d", errProtocol, respCall, call)
	}
	if want := callSignatures[call][1]; string(sig) != want {
		return nil, fmt.Errorf("%w: unexpected signature %q, expected %q", errProtocol, sig, want)
	}
	return d, nil
}

// writeFrame writes a message, see rpc_socket_write_inlock in
// p11-kit/rpc-transport.c.
func writeFrame(w io.Writer, code uint32, body []byte) error {
	frame := make([]byte, 12, 12+len(body))
	binary.BigEndian.PutUint32(frame[0:], code)
	binary.BigEndian.PutUint32(frame[4:], 0)
	binary.BigEndian.PutUint32(frame[8:], uint32(len(body)))
	frame = append(frame, body...)
	_, err := w.Write(frame)
	return err
}

// readFrame reads a message, see rpc_socket_read in
// p11-kit/rpc-transport.c. Options are ignored like p11-kit does.
func readFrame(r io.Reader) (uint32, []byte, error) {
	var header [12]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	code := binary.BigEndian.Uint32(header[0:])
	olen := binary.BigEndian.Uint32(header[4:])
	dlen := binary.BigEndian.Uint32(header[8:])
	if code == 0 {
		return 0, nil, fmt.Errorf("%w: invalid message code 0", errProtocol)
	}
	if olen > maxRPCMessageSize || dlen > maxRPCMessageSize {
		return 0, nil, fmt.Errorf("%w: message too large", errProtocol)
	}
	if _, err := io.CopyN(io.Discard, r, int64(olen)); err != nil {
		return 0, nil, err
	}
	body := make([]byte, dlen)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return code, body, nil
}

func isCKError(err error) bool {
	var ce ckError
	return errors.As(err, &ce)
}

func (c *rpcConn) initialize(ctx context.Context) error {
	req := newRequest(callInitialize)
	req.byteArray([]byte(rpcHandshake))
	req.byte(0)
	req.byteArray([]byte{0})
	d, err := c.call(ctx, req)
	if err != nil {
		if errors.Is(err, ckError(ckrCryptokiAlreadyInitialized)) {
			return nil
		}
		return fmt.Errorf("C_Initialize: %w", err)
	}
	return d.done()
}

func (c *rpcConn) finalize(ctx context.Context) error {
	d, err := c.call(ctx, newRequest(callFinalize))
	if err != nil {
		return fmt.Errorf("C_Finalize: %w", err)
	}
	return d.done()
}

func (c *rpcConn) slotList(ctx context.Context) ([]uint64, error) {
	capacity := uint32(16)
	for range 3 {
		req := newRequest(callGetSlotList)
		req.byte(1)
		req.buffer(capacity)
		d, err := c.call(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("C_GetSlotList: %w", err)
		}
		slots, n := d.ulongArray()
		if err := d.done(); err != nil {
			return nil, err
		}
		if slots != nil || n == 0 {
			return slots, nil
		}
		capacity = n
	}
	return nil, fmt.Errorf("C_GetSlotList: %w", ckError(ckrBufferTooSmall))
}

func (c *rpcConn) tokenInfo(ctx context.Context, slot uint64) (tokenInfo, error) {
	req := newRequest(callGetTokenInfo)
	req.ulong(slot)
	d, err := c.call(ctx, req)
	if err != nil {
		return tokenInfo{}, fmt.Errorf("C_GetTokenInfo: %w", err)
	}
	ti := d.tokenInfo()
	return ti, d.done()
}

func (c *rpcConn) openSession(ctx context.Context, slot uint64) (uint64, error) {
	req := newRequest(callOpenSession)
	req.ulong(slot)
	req.ulong(ckfSerialSession)
	d, err := c.call(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("C_OpenSession: %w", err)
	}
	s := d.ulong()
	return s, d.done()
}

func (c *rpcConn) closeSession(ctx context.Context, session uint64) error {
	req := newRequest(callCloseSession)
	req.ulong(session)
	d, err := c.call(ctx, req)
	if err != nil {
		return fmt.Errorf("C_CloseSession: %w", err)
	}
	return d.done()
}

func (c *rpcConn) login(ctx context.Context, session uint64, pin []byte) error {
	req := newRequest(callLogin)
	req.ulong(session)
	req.ulong(ckuUser)
	req.byteArray(pin)
	d, err := c.call(ctx, req)
	if err != nil {
		if errors.Is(err, ckError(ckrUserAlreadyLoggedIn)) {
			return nil
		}
		return fmt.Errorf("C_Login: %w", err)
	}
	return d.done()
}

func (c *rpcConn) findObjects(ctx context.Context, session uint64, template []attribute) ([]uint64, error) {
	req := newRequest(callFindObjectsInit)
	req.ulong(session)
	req.attributes(template)
	d, err := c.call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("C_FindObjectsInit: %w", err)
	}
	if err := d.done(); err != nil {
		return nil, err
	}

	req = newRequest(callFindObjects)
	req.ulong(session)
	req.buffer(maxFindObjectsResults)
	d, err = c.call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("C_FindObjects: %w", err)
	}
	objects, _ := d.ulongArray()
	if err := d.done(); err != nil {
		return nil, err
	}

	req = newRequest(callFindObjectsFinal)
	req.ulong(session)
	d, err = c.call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("C_FindObjectsFinal: %w", err)
	}
	return objects, d.done()
}

func (c *rpcConn) sign(ctx context.Context, session, key uint64, m mechanism, data []byte) ([]byte, error) {
	req := newRequest(callSignInit)
	req.ulong(session)
	req.mechanism(m)
	req.ulong(key)
	d, err := c.call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("C_SignInit: %w", err)
	}
	if err := d.done(); err != nil {
		return nil, err
	}

	req = newRequest(callSign)
	req.ulong(session)
	req.byteArray(data)
	req.buffer(maxSignatureLength)
	d, err = c.call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("C_Sign: %w", err)
	}
	sig, n := d.byteArray()
	if err := d.done(); err != nil {
		return nil, err
	}
	if sig == nil {
		return nil, fmt.Errorf("C_Sign: signature of %d bytes exceeds buffer: %w", n, ckError(ckrBufferTooSmall))
	}
	return sig, nil
}
