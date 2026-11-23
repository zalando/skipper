//go:build awsnitro && linux

package nitro

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func listenVsockLoopback(t *testing.T) (*os.File, uint32) {
	t.Helper()
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("vsock not supported: %v", err)
	}
	f := os.NewFile(uintptr(fd), "vsock-listener")
	if err := unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_LOCAL, Port: unix.VMADDR_PORT_ANY}); err != nil {
		f.Close()
		t.Skipf("vsock loopback not supported: %v", err)
	}
	require.NoError(t, unix.Listen(fd, 1))
	sa, err := unix.Getsockname(fd)
	require.NoError(t, err)
	return f, sa.(*unix.SockaddrVM).Port
}

func TestDialVsock(t *testing.T) {
	l, port := listenVsockLoopback(t)
	defer l.Close()

	accepted := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		nfd, _, err := unix.Accept(int(l.Fd()))
		if err != nil {
			accepted <- err
			return
		}
		c := os.NewFile(uintptr(nfd), "vsock-accepted")
		defer c.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(c, buf); err != nil {
			accepted <- err
			return
		}
		_, err = c.Write(buf)
		accepted <- err
		<-release
	}()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dialVsock(ctx, unix.VMADDR_CID_LOCAL, port)
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "vsock", conn.RemoteAddr().Network())
	assert.Equal(t, vsockAddr{unix.VMADDR_CID_LOCAL, port}, conn.RemoteAddr())

	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(buf))
	require.NoError(t, <-accepted)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Millisecond)))
	_, err = conn.Read(buf)
	assert.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestDialVsockRefused(t *testing.T) {
	l, port := listenVsockLoopback(t)
	l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := dialVsock(ctx, unix.VMADDR_CID_LOCAL, port)
	assert.Error(t, err)
}
