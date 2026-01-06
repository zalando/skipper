package valkeytest

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
)

type options struct {
	username string
	password string
	image    string
}

func NewTestValkey(t testing.TB) (address string, done func()) {
	t.Helper()
	return newTestValkeyWithOptions(t, options{})
}

func NewTestValkeyWithPassword(t testing.TB, password string) (address string, done func()) {
	t.Helper()
	return newTestValkeyWithOptions(t, options{password: password})
}

func newTestValkeyWithOptions(t testing.TB, opts options) (address string, done func()) {
	t.Helper()
	var args []string
	if opts.password != "" {
		args = append(args, "--requirepass", opts.password)
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
			ExposedPorts: []string{"6379/tcp"},
			Networks:     []string{network.Name},
			WaitingFor: wait.ForAll(
				wait.ForLog("* Ready to accept connections"),
				wait.NewHostPortStrategy(port),
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

	if err := ping(ctx, address, opts.username, opts.password); err != nil {
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

func ping(ctx context.Context, address, username, password string) error {
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
