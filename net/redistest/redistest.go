package redistest

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func NewTestRedis(t testing.TB) (address string, done func()) {
	return NewTestRedisWithPassword(t, "")
}

func NewTestRedisWithPassword(t testing.TB, password string) (address string, done func()) {
	var args []string
	if password != "" {
		args = append(args, "--requirepass", password)
	}

	start := time.Now()

	// first testcontainer start takes longer than subsequent
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:6-alpine",
			Cmd:          args,
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("* Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start redis server: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	address, err = container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get redis address: %v", err)
	}

	t.Logf("Started redis server at %s in %v", address, time.Since(start))

	if err := ping(ctx, address, password); err != nil {
		t.Fatalf("Failed to ping redis server: %v", err)
	}

	done = func() {
		t.Logf("Stopping redis server at %s", address)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("Failed to stop redis: %v", err)
		}
	}
	return
}

func ping(ctx context.Context, address, password string) error {
	rdb := redis.NewClient(&redis.Options{Addr: address, Password: password})
	defer rdb.Close()

	for _, err := rdb.Ping(ctx).Result(); ctx.Err() == nil && err != nil; _, err = rdb.Ping(ctx).Result() {
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}
