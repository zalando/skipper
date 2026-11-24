package valkeytest

import (
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

type options struct {
	username string
	password string
	image    string
	ca       *nettest.CA
}

func NewTestValkey(t testing.TB) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{})
}

func NewTestValkeyWithPassword(t testing.TB, password string) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{password: password})
}

// NewTestValkeySwarmMTLS starts a valkey server that requires mutual TLS. The server
// certificate is signed by ca and valid for the returned address host, clients
// must present a certificate signed by ca. The address is the docker host and
// mapped port instead of the container IP, because the certificate SAN has to
// be known before the container starts.
func NewTestValkeySwarmMTLS(t testing.TB, ca *nettest.CA) (address string, done func()) {
	return newTestValkeyWithOptions(t, options{ca: ca})
}

func newTestValkeyWithOptions(t testing.TB, opts options) (address string, done func()) {
	var args []string
	if opts.password != "" {
		args = append(args, "--requirepass", opts.password)
	}

	logStrategy := wait.ForLog("* Ready to accept connections tcp")

	var hooks []testcontainers.ContainerLifecycleHooks
	if opts.ca != nil {
		args = append(args,
			"--port", "0",
			"--tls-port", "6379",
			"--tls-cert-file", "/certs/server.crt",
			"--tls-key-file", "/certs/server.key",
			"--tls-ca-cert-file", "/certs/ca.crt",
			"--tls-auth-clients", "yes",
		)
		logStrategy = wait.ForLog("* Ready to accept connections tls")
		hooks = append(hooks, testcontainers.ContainerLifecycleHooks{
			PreStarts: []testcontainers.ContainerHook{copyServerCerts(opts.ca)},
		})
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
			Image:          image,
			Cmd:            args,
			ExposedPorts:   []string{"6379/tcp"},
			Networks:       []string{network.Name},
			LifecycleHooks: hooks,
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

	address, err = containerAddress(ctx, container, port, opts.ca != nil)
	if err != nil {
		t.Fatalf("Failed to get valkey container address: %v", err)
	}

	t.Logf("Started valkey server at %s in %v", address, time.Since(start))

	var tlsConfig *tls.Config
	if opts.ca != nil {
		tlsConfig = clientTLSConfig(t, opts.ca)
	}
	if err := ping(ctx, address, opts.username, opts.password, tlsConfig); err != nil {
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

func containerAddress(ctx context.Context, c testcontainers.Container, port nat.Port, useTLS bool) (string, error) {
	if !useTLS {
		ip, err := c.ContainerIP(ctx)
		if err != nil {
			return "", err
		}
		return net.JoinHostPort(ip, port.Port()), nil
	}

	host, err := c.Host(ctx)
	if err != nil {
		return "", err
	}
	mapped, err := c.MappedPort(ctx, string(port))
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, mapped.Port()), nil
}

func copyServerCerts(ca *nettest.CA) testcontainers.ContainerHook {
	return func(ctx context.Context, c testcontainers.Container) error {
		host, err := c.Host(ctx)
		if err != nil {
			return err
		}

		certPEM, keyPEM, err := ca.GenServerCert(host, "127.0.0.1", "localhost")
		if err != nil {
			return err
		}

		for path, content := range map[string][]byte{
			"/certs/server.crt": certPEM,
			"/certs/server.key": keyPEM,
			"/certs/ca.crt":     ca.CertPEM,
		} {
			if err := c.CopyToContainer(ctx, content, path, 0o444); err != nil {
				return fmt.Errorf("failed to copy %s: %w", path, err)
			}
		}
		return nil
	}
}

func clientTLSConfig(t testing.TB, ca *nettest.CA) *tls.Config {
	certPEM, keyPEM := ca.ClientCert(t)
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to load client keypair: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{pair},
	}
}

func ping(ctx context.Context, address, username, password string, tlsConfig *tls.Config) error {
	vdb, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{address},
		Username:    username,
		Password:    password,
		TLSConfig:   tlsConfig,
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
