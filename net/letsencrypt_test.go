package net

import (
	"context"
	"testing"

	"github.com/zalando/skipper/net/redistest"
)

func TestRemoteCache(t *testing.T) {
	redisAddr, done := redistest.NewTestRedis(t)
	defer done()
	if redisAddr == "" {
		t.Fatal("Failed to create redis 1")
	}

	redisAddr2, done2 := redistest.NewTestRedis(t)
	defer done2()
	if redisAddr2 == "" {
		t.Fatal("Failed to create redis 2")
	}

	rc := remoteCache{
		client: NewRedisRingClient(&RedisOptions{
			Addrs: []string{redisAddr, redisAddr2},
		}),
	}
	defer rc.Close()

	if err := rc.Put(context.Background(), "foo", []byte("bar")); err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	if v, err := rc.Get(context.Background(), "foo"); err != nil {
		t.Fatalf("Failed to get: %v", err)
	} else {
		t.Logf("%T %v %s", v, v, v)
		if string(v) != "bar" {
			t.Fatalf("Failed to get result, got: %q", string(v))
		}
	}

	if err := rc.Delete(context.Background(), "foo"); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}
}

func TestInmemoryCache(t *testing.T) {
	rc := &inmemoryCache{}

	if err := rc.Put(context.Background(), "foo", []byte("bar")); err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	if v, err := rc.Get(context.Background(), "foo"); err != nil {
		t.Fatalf("Failed to get: %v", err)
	} else {
		t.Logf("%T %v %s", v, v, v)
	}

	if err := rc.Delete(context.Background(), "foo"); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}
}

func TestLetsencrypt(t *testing.T) {
	validDomain := "szuecs.net"
	if !validateDomain(validDomain) {
		t.Fatalf("Failed to validate valid domain %q", validDomain)
	}
	le := NewLetsencrypt(&inmemoryCache{}, "sandor@szuecs.net", "https://acme-staging-v02.api.letsencrypt.org/directory", []string{validDomain})
	defer le.Close()
	if le.manager.Client != nil {
		dir, err := le.manager.Client.Discover(context.TODO())
		if err != nil {
			t.Fatalf("Failed to discover: %v", err)
		}
		t.Logf("order: %s", dir.OrderURL)
		defer func() {
			if le.manager.Client.HTTPClient != nil {
				le.manager.Client.HTTPClient.CloseIdleConnections()
			}
		}()
	}

	li := le.Listener()
	defer li.Close()
	t.Logf("listener %v", li.Addr())
}
