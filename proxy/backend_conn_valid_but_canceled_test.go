package proxy

import (
	stdlibcontext "context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/quic-go/quic-go"
)

func TestValidConnButCanceled(t *testing.T) {
	ctx, cancel := stdlibcontext.WithCancel(stdlibcontext.Background())
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})

	dc := &skipperDialer{
		f: func(ctx stdlibcontext.Context, network, addr string) (net.Conn, error) {
			cancel()
			return client, nil
		},
	}

	conn, err := dc.DialContext(ctx, "tcp", "localhost:8080")
	if conn != nil {
		conn.Close()
		t.Fatalf("expected nil conn, got %v", conn)
	}
	perr, ok := errors.AsType[*proxyError](err)
	if !ok {
		t.Fatalf("expected *proxyError, got %T: %v", err, err)
	}
	if perr.code != http.StatusGatewayTimeout {
		t.Errorf("expected status %d, got %d", http.StatusGatewayTimeout, perr.code)
	}
	if !errors.Is(perr.err, stdlibcontext.Canceled) {
		t.Errorf("expected error wrapping %v, got %v", stdlibcontext.Canceled, perr.err)
	}
	if perr.DialError() {
		t.Errorf("expected DialError() to be false")
	}
	if _, writeErr := client.Write([]byte("x")); !errors.Is(writeErr, io.ErrClosedPipe) {
		t.Errorf("expected dialed conn to be closed (%v), got %v", io.ErrClosedPipe, writeErr)
	}
}

func TestValidQUICConnButCanceled(t *testing.T) {
	ctx, cancel := stdlibcontext.WithCancel(stdlibcontext.Background())

	dc := &skipperDialer{
		fQuic: func(ctx stdlibcontext.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			cancel()
			return nil, nil
		},
	}

	conn, err := dc.DialQUIC(ctx, "localhost:8080", nil, nil)
	if conn != nil {
		conn.CloseWithError(0, "unexpected conn")
		t.Fatalf("expected nil conn, got %v", conn)
	}
	perr, ok := errors.AsType[*proxyError](err)
	if !ok {
		t.Fatalf("expected *proxyError, got %T: %v", err, err)
	}
	if perr.code != http.StatusGatewayTimeout {
		t.Errorf("expected status %d, got %d", http.StatusGatewayTimeout, perr.code)
	}
	if !errors.Is(perr.err, stdlibcontext.Canceled) {
		t.Errorf("expected error wrapping %v, got %v", stdlibcontext.Canceled, perr.err)
	}
	if perr.DialError() {
		t.Errorf("expected DialError() to be false")
	}
}
