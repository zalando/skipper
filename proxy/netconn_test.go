// The code taken from https://go.dev/src/net/http/netconn_test.go
// See also:
// - https://github.com/golang/go/issues/76608
// - https://github.com/golang/go/issues/14200

package proxy_test

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing/synctest"
	"time"
)

func fakeNetListen() *fakeNetListener {
	li := &fakeNetListener{
		setc:    make(chan struct{}, 1),
		unsetc:  make(chan struct{}, 1),
		addr:    netip.MustParseAddrPort("127.0.0.1:8000"),
		locPort: 10000,
	}
	li.unsetc <- struct{}{}
	return li
}

type fakeNetListener struct {
	setc, unsetc chan struct{}
	queue        []net.Conn
	closed       bool
	addr         netip.AddrPort
	locPort      uint16

	onDial  func()
	onClose func(*fakeNetConn)

	trackConns bool
	conns      []*fakeNetConn
}

func (li *fakeNetListener) lock() {
	select {
	case <-li.setc:
	case <-li.unsetc:
	}
}

func (li *fakeNetListener) unlock() {
	if li.closed || len(li.queue) > 0 {
		li.setc <- struct{}{}
	} else {
		li.unsetc <- struct{}{}
	}
}

func (li *fakeNetListener) connect() *fakeNetConn {
	if li.onDial != nil {
		li.onDial()
	}
	li.lock()
	defer li.unlock()
	locAddr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), li.locPort)
	li.locPort++
	c0, c1 := fakeNetPipe(li.addr, locAddr)
	c0.onClose = li.onClose
	c1.onClose = li.onClose
	li.queue = append(li.queue, c0)
	if li.trackConns {
		li.conns = append(li.conns, c0)
	}
	return c1
}

func (li *fakeNetListener) Accept() (net.Conn, error) {
	<-li.setc
	defer li.unlock()
	if li.closed {
		return nil, net.ErrClosed
	}
	c := li.queue[0]
	li.queue = li.queue[1:]
	return c, nil
}

func (li *fakeNetListener) Close() error {
	li.lock()
	defer li.unlock()
	li.closed = true
	return nil
}

func (li *fakeNetListener) Addr() net.Addr {
	return net.TCPAddrFromAddrPort(li.addr)
}

func fakeNetPipe(s1ap, s2ap netip.AddrPort) (r, w *fakeNetConn) {
	s1addr := net.TCPAddrFromAddrPort(s1ap)
	s2addr := net.TCPAddrFromAddrPort(s2ap)
	s1 := newSynctestNetConnHalf(s1addr)
	s2 := newSynctestNetConnHalf(s2addr)
	c1 := &fakeNetConn{loc: s1, rem: s2}
	c2 := &fakeNetConn{loc: s2, rem: s1}
	c1.peer = c2
	c2.peer = c1
	return c1, c2
}

type fakeNetConn struct {
	loc, rem *fakeNetConnHalf
	autoWait bool
	peer     *fakeNetConn
	onClose  func(*fakeNetConn)
}

func (c *fakeNetConn) Read(b []byte) (n int, err error) {
	if c.autoWait {
		synctest.Wait()
	}
	return c.loc.read(b)
}

func (c *fakeNetConn) Peek() []byte {
	if c.autoWait {
		synctest.Wait()
	}
	return c.loc.peek()
}

func (c *fakeNetConn) Write(b []byte) (n int, err error) {
	if c.autoWait {
		defer synctest.Wait()
	}
	return c.rem.write(b)
}

func (c *fakeNetConn) IsClosedByPeer() bool {
	if c.autoWait {
		synctest.Wait()
	}
	c.rem.lock()
	defer c.rem.unlock()
	return c.rem.readErr == net.ErrClosed
}

func (c *fakeNetConn) Close() error {
	if c.onClose != nil {
		c.onClose(c)
	}
	c.loc.lock()
	c.loc.writeErr = net.ErrClosed
	c.loc.readErr = net.ErrClosed
	c.loc.buf.Reset()
	c.loc.unlock()

	c.rem.lock()
	if c.rem.readErr != nil {
		c.rem.readErr = io.EOF
	}

	c.rem.rctx.setDeadline(time.Now())

	c.rem.unlock()
	if c.autoWait {
		synctest.Wait()
	}
	return nil
}

func (c *fakeNetConn) LocalAddr() net.Addr {
	return c.loc.addr
}

func (c *fakeNetConn) RemoteAddr() net.Addr {
	return c.rem.addr
}

func (c *fakeNetConn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

func (c *fakeNetConn) SetReadDeadline(t time.Time) error {
	c.loc.rctx.setDeadline(t)
	return nil
}

func (c *fakeNetConn) SetWriteDeadline(t time.Time) error {
	c.rem.wctx.setDeadline(t)
	return nil
}

func (c *fakeNetConn) SetReadBufferSize(size int) {
	c.loc.setReadBufferSize(size)
}

type fakeNetConnHalf struct {
	addr       net.Addr
	rctx, wctx deadlineContext
	lockr      chan struct{}
	lockw      chan struct{}
	lockrw     chan struct{}
	lockc      chan struct{}
	bufMax     int
	buf        bytes.Buffer
	readErr    error
	writeErr   error
}

func newSynctestNetConnHalf(addr net.Addr) *fakeNetConnHalf {
	h := &fakeNetConnHalf{
		addr:   addr,
		lockw:  make(chan struct{}, 1),
		lockr:  make(chan struct{}, 1),
		lockrw: make(chan struct{}, 1),
		lockc:  make(chan struct{}, 1),
		bufMax: math.MaxInt,
	}
	h.unlock()
	return h
}

func (h *fakeNetConnHalf) lock() {
	select {
	case <-h.lockw:
	case <-h.lockr:
	case <-h.lockrw:
	case <-h.lockc:
	}
}

func (h *fakeNetConnHalf) unlock() {
	canRead := h.readErr != nil || h.buf.Len() > 0
	canWrite := h.writeErr != nil || h.bufMax > h.buf.Len()
	switch {
	case canRead && canWrite:
		h.lockrw <- struct{}{}
	case canRead:
		h.lockr <- struct{}{}
	case canWrite:
		h.lockw <- struct{}{}
	default:
		h.lockc <- struct{}{}
	}
}

func (h *fakeNetConnHalf) waitAndLockForRead() error {
	select {
	case <-h.lockr:
		return nil
	case <-h.lockrw:
		return nil
	default:
	}
	ctx := h.rctx.context()
	select {
	case <-h.lockr:
		return nil
	case <-h.lockrw:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (h *fakeNetConnHalf) waitAndLockForWrite() error {
	select {
	case <-h.lockw:
		return nil
	case <-h.lockrw:
		return nil
	default:
	}
	ctx := h.wctx.context()
	select {
	case <-h.lockw:
		return nil
	case <-h.lockrw:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (h *fakeNetConnHalf) peek() []byte {
	h.lock()
	defer h.unlock()
	return h.buf.Bytes()
}

func (h *fakeNetConnHalf) read(b []byte) (n int, err error) {
	if err := h.waitAndLockForRead(); err != nil {
		return 0, err
	}
	defer h.unlock()
	if h.buf.Len() == 0 && h.readErr != nil {
		return 0, h.readErr
	}
	return h.buf.Read(b)
}

func (h *fakeNetConnHalf) setReadBufferSize(size int) {
	h.lock()
	defer h.unlock()
	h.bufMax = size
}

func (h *fakeNetConnHalf) write(b []byte) (n int, err error) {
	for n < len(b) {
		nn, err := h.writePartial(b[n:])
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (h *fakeNetConnHalf) writePartial(b []byte) (n int, err error) {
	if err := h.waitAndLockForWrite(); err != nil {
		return 0, err
	}
	defer h.unlock()
	if h.writeErr != nil {
		return 0, h.writeErr
	}
	writeMax := h.bufMax - h.buf.Len()
	if writeMax < len(b) {
		b = b[:writeMax]
	}
	return h.buf.Write(b)
}

type deadlineContext struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelCauseFunc
	timer  *time.Timer
}

func (t *deadlineContext) context() context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ctx == nil {
		t.ctx, t.cancel = context.WithCancelCause(context.Background())
	}
	return t.ctx
}

func (t *deadlineContext) setDeadline(deadline time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ctx == nil || t.cancel == nil {
		t.ctx, t.cancel = context.WithCancelCause(context.Background())
	}
	if t.timer != nil {
		t.timer.Stop()
	}
	if deadline.IsZero() {
		return
	}
	now := time.Now()
	if !deadline.After(now) {
		t.cancel(os.ErrDeadlineExceeded)
		t.cancel = nil
		return
	}
	if t.timer != nil {
		t.timer.Reset(deadline.Sub(now))
		return
	}
	t.timer = time.AfterFunc(deadline.Sub(now), func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.cancel(os.ErrDeadlineExceeded)
		t.cancel = nil
	})
}
