//go:build awsnitro && linux

package nitro

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func dialEndpoint(ctx context.Context, ep parsedEndpoint) (net.Conn, error) {
	switch ep.scheme {
	case "unix":
		var d net.Dialer
		return d.DialContext(ctx, "unix", ep.path)
	case "vsock":
		return dialVsock(ctx, ep.cid, ep.port)
	default:
		return nil, fmt.Errorf("aws/nitro: unsupported scheme %q", ep.scheme)
	}
}

func dialVsock(ctx context.Context, cid, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("aws/nitro: vsock socket: %w", err)
	}

	sa := &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}

	err = unix.Connect(fd, sa)
	if err != nil && !errors.Is(err, unix.EINPROGRESS) {
		unix.Close(fd)
		return nil, fmt.Errorf("aws/nitro: vsock connect cid=%d port=%d: %w", cid, port, err)
	}

	if errors.Is(err, unix.EINPROGRESS) {
		if err := waitVsockConnect(ctx, fd); err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("aws/nitro: vsock connect cid=%d port=%d: %w", cid, port, err)
		}
	}

	f := os.NewFile(uintptr(fd), fmt.Sprintf("vsock://%d:%d", cid, port))
	conn, err := net.FileConn(f)
	// net.FileConn duplicates fd; close the original os.File wrapper.
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("aws/nitro: vsock wrap conn: %w", err)
	}
	return conn, nil
}

func waitVsockConnect(ctx context.Context, fd int) error {
	pollFds := []unix.PollFd{{
		Fd:     int32(fd),
		Events: unix.POLLOUT,
	}}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		timeoutMs := 100
		if deadline, ok := ctx.Deadline(); ok {
			rem := time.Until(deadline)
			if rem <= 0 {
				return context.DeadlineExceeded
			}
			if ms := int(rem / time.Millisecond); ms < timeoutMs {
				timeoutMs = max(ms, 1)
			}
		}

		n, err := unix.Poll(pollFds, timeoutMs)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		soErr, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
		if err != nil {
			return err
		}
		if soErr != 0 {
			return syscall.Errno(soErr)
		}
		return nil
	}
}
