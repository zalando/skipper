package redistest

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type options struct {
	password string
	image    string
}

func NewTestRedis(t testing.TB) (address string, done func()) {
	t.Helper()
	return newTestRedisWithOptions(t, options{})
}

func NewTestRedisWithPassword(t testing.TB, password string) (address string, done func()) {
	t.Helper()
	return newTestRedisWithOptions(t, options{password: password})
}

func newTestRedisWithOptions(t testing.TB, opts options) (address string, done func()) {
	t.Helper()
	var args []string
	if opts.password != "" {
		args = append(args, "--requirepass", opts.password)
	}

	start := time.Now()

	network, err := redisTestNetwork.acquire()
	if err != nil {
		t.Fatalf("Failed to get redis test network: %v", err)
	}

	// first testcontainer start takes longer than subsequent
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	port, err := nat.NewPort("tcp", "6379")
	if err != nil {
		t.Fatalf("Failed to get new nat port: %v", err)
	}

	image := "redis:7-alpine"
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
		t.Fatalf("Failed to start redis server: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ip, err := container.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("Failed to get redis container ip: %v", err)
	}
	address = net.JoinHostPort(ip, "6379")

	t.Logf("Started redis server at %s in %v", address, time.Since(start))

	if err := ping(ctx, address, opts.password); err != nil {
		t.Fatalf("Failed to ping redis server: %v", err)
	}

	done = func() {
		t.Logf("Stopping redis server at %s", address)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("Failed to stop redis: %v", err)
		}

		redisTestNetwork.release()
	}
	return
}

func ping(ctx context.Context, address, password string) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		// https://github.com/redis/go-redis/issues/3536#issuecomment-3499924405
		// Explicitly disable maintenance notifications
		// This prevents the client from sending CLIENT MAINT_NOTIFICATIONS ON
		MaintNotificationsConfig: &maintnotifications.Config{
			Mode: maintnotifications.ModeDisabled,
		}})
	defer rdb.Close()

	for _, err := rdb.Ping(ctx).Result(); ctx.Err() == nil && err != nil; _, err = rdb.Ping(ctx).Result() {
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}
