//go:build awsnitro && linux

package nitro

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type vsockAddr struct {
	cid, port uint32
}

func (a vsockAddr) Network() string { return "vsock" }
func (a vsockAddr) String() string  { return fmt.Sprintf("vm(%d):%d", a.cid, a.port) }

type vsockConn struct {
	*os.File
	local, remote vsockAddr
}

func (c *vsockConn) LocalAddr() net.Addr  { return c.local }
func (c *vsockConn) RemoteAddr() net.Addr { return c.remote }

func dialVsock(ctx context.Context, cid, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create vsock socket: %w", err)
	}

	// A non-blocking fd is registered with the runtime network poller,
	// which provides deadlines, see os.NewFile.
	f := os.NewFile(uintptr(fd), "vsock")
	conn, err := connectVsock(ctx, f, cid, port)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to connect to vsock %s: %w", vsockAddr{cid, port}, err)
	}
	return conn, nil
}

func connectVsock(ctx context.Context, f *os.File, cid, port uint32) (net.Conn, error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := f.SetWriteDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stop := context.AfterFunc(ctx, func() { f.SetWriteDeadline(time.Unix(1, 0)) })
	defer stop()

	sa := &unix.SockaddrVM{CID: cid, Port: port}
	started := false
	var connErr error
	// Same approach as Go's net package for non-blocking connect,
	// see net/fd_unix.go (*netFD).connect.
	err = rc.Write(func(fd uintptr) bool {
		if !started {
			started = true
			connErr = unix.Connect(int(fd), sa)
			return !errors.Is(connErr, unix.EINPROGRESS) && !errors.Is(connErr, unix.EINTR)
		}
		v, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_ERROR)
		if err != nil {
			connErr = err
			return true
		}
		switch e := unix.Errno(v); e {
		case unix.EINPROGRESS, unix.EALREADY, unix.EINTR:
			return false
		case 0:
			if _, err := unix.Getpeername(int(fd)); errors.Is(err, unix.ENOTCONN) {
				return false
			}
			connErr = nil
			return true
		default:
			connErr = e
			return true
		}
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if connErr != nil {
		return nil, connErr
	}
	if err := f.SetWriteDeadline(time.Time{}); err != nil {
		return nil, err
	}

	conn := &vsockConn{File: f, remote: vsockAddr{cid, port}}
	if err := rc.Control(func(fd uintptr) {
		if sa, err := unix.Getsockname(int(fd)); err == nil {
			if vm, ok := sa.(*unix.SockaddrVM); ok {
				conn.local = vsockAddr{vm.CID, vm.Port}
			}
		}
	}); err != nil {
		return nil, err
	}
	return conn, nil
}
