package redistest

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func NewTestRedis(t *testing.T) (address string, done func()) {
	return NewTestRedisWithPassword(t, "")
}

func NewTestRedisWithPassword(t *testing.T, password string) (address string, done func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address = l.Addr().String()
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()

	args := []string{"--port", port}
	if password != "" {
		args = append(args, "--requirepass", password)
	}

	ctx, stop := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "redis-server", args...) // #nosec
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start redis server: %v", err)
	}

	if err := ping(address, password); err != nil {
		t.Fatalf("Failed to ping redis server: %v", err)
	}

	t.Logf("Started redis server at %s", address)
	done = func() {
		t.Logf("Stopping redis server at %s", address)
		stop()
		cmd.Wait()
	}
	return
}

func ping(address, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: address, Password: password})
	defer rdb.Close()

	for _, err := rdb.Ping(ctx).Result(); ctx.Err() == nil && err != nil; _, err = rdb.Ping(ctx).Result() {
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}
