// The code taken from https://go.dev/src/net/http/netconn_test.go
// See also:
// - https://github.com/golang/go/issues/76608
// - https://github.com/golang/go/issues/14200

package nettest

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"testing/synctest"
	"time"
)

// FakeNetListen creates an in-memory, synthetic net.Listener for testing.
func FakeNetListen() *FakeNetListener {
	return NewFakeNetListener()
}

// NewFakeNetListener returns an initialized *FakeNetListener.
func NewFakeNetListener() *FakeNetListener {
	li := &FakeNetListener{
		setc:    make(chan struct{}, 1),
		unsetc:  make(chan struct{}, 1),
		addr:    netip.MustParseAddrPort("127.0.0.1:8000"),
		locPort: 10000,
	}
	li.unsetc <- struct{}{}
	return li
}

// FakeNetListener is an in-memory net.Listener that queues incoming connections.
type FakeNetListener struct {
	setc, unsetc chan struct{}
	queue        []net.Conn
	closed       bool
	addr         netip.AddrPort
	locPort      uint16

	OnDial  func()                  // called when making a new connection
	OnClose func(conn *FakeNetConn) // called when closing a connection

	TrackConns bool // set this to record all created conns
	Conns      []*FakeNetConn
}

var _ net.Listener = &FakeNetListener{}

func (li *FakeNetListener) lock() {
	select {
	case <-li.setc:
	case <-li.unsetc:
	}
}

func (li *FakeNetListener) unlock() {
	if li.closed || len(li.queue) > 0 {
		li.setc <- struct{}{}
	} else {
		li.unsetc <- struct{}{}
	}
}

// Connect creates a client-side net.Conn connected to this listener.
func (li *FakeNetListener) Connect() (net.Conn, error) {
	return li.ConnectConn(), nil
}

// ConnectConn creates and returns the client-side *FakeNetConn endpoint.
func (li *FakeNetListener) ConnectConn() *FakeNetConn {
	if li.OnDial != nil {
		li.OnDial()
	}
	li.lock()
	defer li.unlock()
	locAddr := netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), li.locPort)
	li.locPort++
	c0, c1 := FakeNetPipe(li.addr, locAddr)
	c0.onClose = li.OnClose
	c1.onClose = li.OnClose
	li.queue = append(li.queue, c0)
	if li.TrackConns {
		li.Conns = append(li.Conns, c0)
	}
	return c1
}

// Accept waits for and returns the next connection to the listener.
func (li *FakeNetListener) Accept() (net.Conn, error) {
	<-li.setc
	defer li.unlock()
	if li.closed {
		return nil, net.ErrClosed
	}
	c := li.queue[0]
	li.queue = li.queue[1:]
	return c, nil
}

// Close closes the listener.
func (li *FakeNetListener) Close() error {
	li.lock()
	defer li.unlock()
	li.closed = true
	return nil
}

// Addr returns the listener's network address.
func (li *FakeNetListener) Addr() net.Addr {
	return net.TCPAddrFromAddrPort(li.addr)
}

// DialContext creates a DialContext func compatible with http.Transport.
func (li *FakeNetListener) DialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return li.Connect()
	}
}

// NewTestTransport returns an http.Transport configured to dial through the given FakeNetListener.
func NewTestTransport(li *FakeNetListener) *http.Transport {
	return &http.Transport{
		DialContext: li.DialContext(),
	}
}

// FakeNetPipe creates an in-memory, full duplex network connection.
//
// Unlike net.Pipe, the connection is not synchronous.
// Writes are made to a buffer, and return immediately.
// By default, the buffer size is unlimited.
func FakeNetPipe(s1ap, s2ap netip.AddrPort) (r, w *FakeNetConn) {
	s1addr := net.TCPAddrFromAddrPort(s1ap)
	s2addr := net.TCPAddrFromAddrPort(s2ap)
	s1 := newSynctestNetConnHalf(s1addr)
	s2 := newSynctestNetConnHalf(s2addr)
	c1 := &FakeNetConn{loc: s1, rem: s2}
	c2 := &FakeNetConn{loc: s2, rem: s1}
	c1.peer = c2
	c2.peer = c1
	return c1, c2
}

// A FakeNetConn is one endpoint of the connection created by FakeNetPipe.
type FakeNetConn struct {
	// local and remote connection halves.
	// Each half contains a buffer.
	// Reads pull from the local buffer, and writes push to the remote buffer.
	loc, rem *fakeNetConnHalf

	// When set, synctest.Wait is automatically called before reads and after writes.
	AutoWait bool

	// peer is the other endpoint.
	peer *FakeNetConn

	onClose func(*FakeNetConn) // called when closing
}

var _ net.Conn = &FakeNetConn{}

// Read reads data from the connection.
func (c *FakeNetConn) Read(b []byte) (n int, err error) {
	if c.AutoWait {
		synctest.Wait()
	}
	return c.loc.read(b)
}

// Peek returns the available unread read buffer,
// without consuming its contents.
func (c *FakeNetConn) Peek() []byte {
	if c.AutoWait {
		synctest.Wait()
	}
	return c.loc.peek()
}

// Write writes data to the connection.
func (c *FakeNetConn) Write(b []byte) (n int, err error) {
	if c.AutoWait {
		defer synctest.Wait()
	}
	return c.rem.write(b)
}

// IsClosedByPeer reports whether the peer has closed its end of the connection.
func (c *FakeNetConn) IsClosedByPeer() bool {
	if c.AutoWait {
		synctest.Wait()
	}
	c.rem.lock()
	defer c.rem.unlock()
	// If the remote half of the conn is returning ErrClosed,
	// the peer has closed the connection.
	return c.rem.readErr == net.ErrClosed
}

// Close closes the connection.
func (c *FakeNetConn) Close() error {
	if c.onClose != nil {
		c.onClose(c)
	}
	// Local half of the conn is now closed.
	c.loc.lock()
	c.loc.writeErr = net.ErrClosed
	c.loc.readErr = net.ErrClosed
	c.loc.buf.Reset()
	c.loc.unlock()
	// Remote half of the connection reads EOF after reading any remaining data.
	c.rem.lock()
	if c.rem.readErr != nil {
		c.rem.readErr = io.EOF
	}

	c.rem.rctx.setDeadline(time.Now()) // expire immediately

	c.rem.unlock()
	if c.AutoWait {
		synctest.Wait()
	}
	return nil
}

// LocalAddr returns the (fake) local network address.
func (c *FakeNetConn) LocalAddr() net.Addr {
	return c.loc.addr
}

// RemoteAddr returns the (fake) remote network address.
func (c *FakeNetConn) RemoteAddr() net.Addr {
	return c.rem.addr
}

// SetDeadline sets the read and write deadlines for the connection.
func (c *FakeNetConn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline sets the read deadline for the connection.
func (c *FakeNetConn) SetReadDeadline(t time.Time) error {
	c.loc.rctx.setDeadline(t)
	return nil
}

// SetWriteDeadline sets the write deadline for the connection.
func (c *FakeNetConn) SetWriteDeadline(t time.Time) error {
	c.rem.wctx.setDeadline(t)
	return nil
}

// SetReadBufferSize sets the read buffer limit for the connection.
// Writes by the peer will block so long as the buffer is full.
func (c *FakeNetConn) SetReadBufferSize(size int) {
	c.loc.setReadBufferSize(size)
}

// fakeNetConnHalf is one data flow in the connection created by FakeNetPipe.
// Each half contains a buffer. Writes to the half push to the buffer, and reads pull from it.
type fakeNetConnHalf struct {
	addr net.Addr

	// Read and write timeouts.
	rctx, wctx deadlineContext

	// A half can be readable and/or writable.
	//
	// These four channels act as a lock,
	// and allow waiting for readability/writability.
	// When the half is unlocked, exactly one channel contains a value.
	// When the half is locked, all channels are empty.
	lockr  chan struct{} // readable
	lockw  chan struct{} // writable
	lockrw chan struct{} // readable and writable
	lockc  chan struct{} // neither readable nor writable

	bufMax   int // maximum buffer size
	buf      bytes.Buffer
	readErr  error // error returned by reads
	writeErr error // error returned by writes
}

func newSynctestNetConnHalf(addr net.Addr) *fakeNetConnHalf {
	h := &fakeNetConnHalf{
		addr:   addr,
		lockw:  make(chan struct{}, 1),
		lockr:  make(chan struct{}, 1),
		lockrw: make(chan struct{}, 1),
		lockc:  make(chan struct{}, 1),
		bufMax: math.MaxInt, // unlimited
	}
	h.unlock()
	return h
}

// lock locks h.
func (h *fakeNetConnHalf) lock() {
	select {
	case <-h.lockw: // writable
	case <-h.lockr: // readable
	case <-h.lockrw: // readable and writable
	case <-h.lockc: // neither readable nor writable
	}
}

// unlock unlocks h.
func (h *fakeNetConnHalf) unlock() {
	canRead := h.readErr != nil || h.buf.Len() > 0
	canWrite := h.writeErr != nil || h.bufMax > h.buf.Len()
	switch {
	case canRead && canWrite:
		h.lockrw <- struct{}{} // readable and writable
	case canRead:
		h.lockr <- struct{}{} // readable
	case canWrite:
		h.lockw <- struct{}{} // writable
	default:
		h.lockc <- struct{}{} // neither readable nor writable
	}
}

// waitAndLockForRead waits until h is readable and locks it.
func (h *fakeNetConnHalf) waitAndLockForRead() error {
	// First a non-blocking select to see if we can make immediate progress.
	// This permits using a canceled context for a non-blocking operation.
	select {
	case <-h.lockr:
		return nil // readable
	case <-h.lockrw:
		return nil // readable and writable
	default:
	}
	ctx := h.rctx.context()
	select {
	case <-h.lockr:
		return nil // readable
	case <-h.lockrw:
		return nil // readable and writable
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// waitAndLockForWrite waits until h is writable and locks it.
func (h *fakeNetConnHalf) waitAndLockForWrite() error {
	// First a non-blocking select to see if we can make immediate progress.
	// This permits using a canceled context for a non-blocking operation.
	select {
	case <-h.lockw:
		return nil // writable
	case <-h.lockrw:
		return nil // readable and writable
	default:
	}
	ctx := h.wctx.context()
	select {
	case <-h.lockw:
		return nil // writable
	case <-h.lockrw:
		return nil // readable and writable
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

// deadlineContext converts a changeable deadline (as in net.Conn.SetDeadline) into a Context.
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
