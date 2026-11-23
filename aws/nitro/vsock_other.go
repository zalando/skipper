//go:build awsnitro && !linux

package nitro

import (
	"context"
	"fmt"
	"net"
)

func dialEndpoint(ctx context.Context, ep parsedEndpoint) (net.Conn, error) {
	switch ep.scheme {
	case "unix":
		var d net.Dialer
		return d.DialContext(ctx, "unix", ep.path)
	case "vsock":
		return nil, fmt.Errorf("%w: AF_VSOCK requires Linux", ErrNotSupported)
	default:
		return nil, fmt.Errorf("aws/nitro: unsupported scheme %q", ep.scheme)
	}
}
