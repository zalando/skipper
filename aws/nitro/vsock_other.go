//go:build awsnitro && !linux

package nitro

import (
	"context"
	"net"
)

func dialVsock(context.Context, uint32, uint32) (net.Conn, error) {
	return nil, ErrNotSupported
}
