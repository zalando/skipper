package valkeytest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
	"github.com/zalando/skipper/net/nettest"
)

// testCerts holds PEM-encoded material for a valkey mutual TLS test.
// CA PEM verifies both sides, ServerCertPEM/ServerKeyPEM are presented by the
// valkey container, and ClientCertPEM/ClientKeyPEM are presented by Skipper.
type testCerts struct {
	caPEM         []byte
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

// newTestCerts generates a CA, a server certificate (serverAuth, with the given
// hosts as SANs, defaulting to 127.0.0.1) and a client certificate (clientAuth),
// all PEM-encoded and signed by the CA. The ping dials the container by IP, so
// pingTLS pins ServerName to 127.0.0.1 to match the server certificate SAN.
func newTestCerts(t testing.TB, hosts ...string) *testCerts {
	t.Helper()

	ca := nettest.NewCA(t)
	serverCertPEM, serverKeyPEM := ca.ServerCert(t, hosts...)
	clientCertPEM, clientKeyPEM := ca.ClientCert(t)

	return &testCerts{
		caPEM:         ca.CertPEM,
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
		clientCertPEM: clientCertPEM,
		clientKeyPEM:  clientKeyPEM,
	}
}

type options struct {
	username string
	password string
	image    string
	tls      *testCerts
}

func NewTestValkey(t testing.TB) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{})
}

func NewTestValkeyWithPassword(t testing.TB, password string) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{password: password})
}

func newTestValkeyWithOptions(t testing.TB, opts options) (address string, done func()) {
	var args []string
	if opts.password != "" {
		args = append(args, "--requirepass", opts.password)
	}

	logStrategy := wait.ForLog("* Ready to accept connections tcp")

	var files []testcontainers.ContainerFile
	if opts.tls != nil {
		files = []testcontainers.ContainerFile{
			{Reader: bytes.NewReader(opts.tls.serverCertPEM), ContainerFilePath: "/certs/server.crt", FileMode: 0o444},
			{Reader: bytes.NewReader(opts.tls.serverKeyPEM), ContainerFilePath: "/certs/server.key", FileMode: 0o444},
			{Reader: bytes.NewReader(opts.tls.caPEM), ContainerFilePath: "/certs/ca.crt", FileMode: 0o444},
		}
		args = append(args,
			"--port", "0",
			"--tls-port", "6379",
			"--tls-cert-file", "/certs/server.crt",
			"--tls-key-file", "/certs/server.key",
			"--tls-ca-cert-file", "/certs/ca.crt",
			"--tls-auth-clients", "yes",
		)
		logStrategy = wait.ForLog("* Ready to accept connections tls")
	}

	start := time.Now()

	network, err := valkeyTestNetwork.acquire()
	if err != nil {
		t.Fatalf("Failed to get valkey test network: %v", err)
	}

	// first testcontainer start takes longer than subsequent
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	port, err := nat.NewPort("tcp", "6379")
	if err != nil {
		t.Fatalf("Failed to get new nat port: %v", err)
	}

	image := "valkey/valkey:9-alpine3.23"
	if opts.image != "" {
		image = opts.image
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			Cmd:          args,
			Files:        files,
			ExposedPorts: []string{"6379/tcp"},
			Networks:     []string{network.Name},
			WaitingFor: wait.ForAll(
				logStrategy,
				wait.NewHostPortStrategy(port.Port()),
			),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start valkey server: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ip, err := container.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("Failed to get valkey container ip: %v", err)
	}
	address = net.JoinHostPort(ip, "6379")

	t.Logf("Started valkey server at %s in %v", address, time.Since(start))

	if err := ping(ctx, address, &opts); err != nil {
		t.Fatalf("Failed to ping valkey server: %v", err)
	}

	done = func() {
		t.Logf("Stopping valkey server at %s", address)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("Failed to stop valkey: %v", err)
		}

		valkeyTestNetwork.release()
	}
	return
}

func ping(ctx context.Context, address string, opts *options) error {
	if opts.tls != nil {
		return pingTLS(ctx, address, opts.username, opts.password, opts.tls)
	}
	return pingPlain(ctx, address, opts.username, opts.password)
}

func pingPlain(ctx context.Context, address, username, password string) error {
	vdb, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{address},
		Username:    username,
		Password:    password,
	})
	if err != nil {
		return err
	}

	defer vdb.Close()

	for res := vdb.Do(ctx, vdb.B().Ping().Build()); ctx.Err() == nil && res.Error() != nil; res = vdb.Do(ctx, vdb.B().Ping().Build()) {
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}

func newValkeyWithTLS(t *testing.T) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{
		tls: newTestCerts(t),
	})
}

func pingTLS(ctx context.Context, address, username, password string, tc *testCerts) error {
	clientPair, err := tls.X509KeyPair(tc.clientCertPEM, tc.clientKeyPEM)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(tc.caPEM) {
		return fmt.Errorf("failed to append CA cert")
	}

	vdb, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{address},
		Username:    username,
		Password:    password,
		TLSConfig: &tls.Config{
			ServerName:   "127.0.0.1",
			RootCAs:      pool,
			Certificates: []tls.Certificate{clientPair},
		},
	})
	if err != nil {
		return err
	}

	defer vdb.Close()

	for res := vdb.Do(ctx, vdb.B().Ping().Build()); ctx.Err() == nil && res.Error() != nil; res = vdb.Do(ctx, vdb.B().Ping().Build()) {
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}
