package net

import (
	"context"
	"crypto/x509"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/redis/go-redis/v9"
	"github.com/zalando/skipper/net/nettest"
	"github.com/zalando/skipper/net/redistest"
)

func newRedisSwarmMTLSOptions(addrs []string, enable bool, pool *x509.CertPool, client swarmMTLSClient) *RedisOptions {
	return &RedisOptions{
		Addrs:          addrs,
		EnableMTLS:     enable,
		CA:             pool,
		ClientCertFile: client.certFile,
		ClientKeyFile:  client.keyFile,
		DialTimeout:    time.Second,
		ReadTimeout:    time.Second,
		WriteTimeout:   time.Second,
	}
}

func redisSetGet(ctx context.Context, opt *RedisOptions) error {
	cli := NewRedisRingClient(opt)
	defer cli.Close()

	if _, err := cli.Set(ctx, "k", "v"); err != nil {
		return err
	}
	_, err := cli.Get(ctx, "k")
	return err
}

func TestRedisSwarmMTLS(t *testing.T) {
	ca := nettest.NewCA(t)
	otherCA := nettest.NewCA(t)
	addr, done := redistest.NewTestRedisSwarmMTLS(t, ca)
	defer done()

	trusted := newSwarmMTLSClient(t, ca)
	untrusted := newSwarmMTLSClient(t, otherCA)

	for _, tt := range []struct {
		name    string
		enable  bool
		pool    *x509.CertPool
		client  swarmMTLSClient
		wantErr string
	}{
		{
			name:   "swarm mTLS",
			enable: true,
			pool:   trusted.pool,
			client: trusted,
		},
		{
			name:    "untrusted server certificate",
			enable:  true,
			pool:    untrusted.pool,
			client:  trusted,
			wantErr: "certificate signed by unknown authority",
		},
		{
			name:    "untrusted client certificate",
			enable:  true,
			pool:    trusted.pool,
			client:  untrusted,
			wantErr: "unknown certificate authority",
		},
		{
			name:    "missing client certificate",
			enable:  true,
			pool:    trusted.pool,
			wantErr: "swarm mTLS requires a client certificate and key file",
		},
		{
			name:    "swarm mTLS disabled",
			enable:  false,
			pool:    trusted.pool,
			client:  trusted,
			wantErr: "EOF|connection reset by peer",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := redisSetGet(ctx, newRedisSwarmMTLSOptions([]string{addr}, tt.enable, tt.pool, tt.client))
			if tt.wantErr != "" {
				if err == nil || !regexp.MustCompile(tt.wantErr).MatchString(err.Error()) {
					t.Fatalf("Failed to get error %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Failed to use swarm mTLS: %v", err)
			}
		})
	}
}

func TestRedisSwarmMTLSSetAddrs(t *testing.T) {
	ca := nettest.NewCA(t)
	addr1, done1 := redistest.NewTestRedisSwarmMTLS(t, ca)
	defer done1()
	addr2, done2 := redistest.NewTestRedisSwarmMTLS(t, ca)
	defer done2()

	client := newSwarmMTLSClient(t, ca)
	cli := NewRedisRingClient(newRedisSwarmMTLSOptions([]string{addr1}, true, client.pool, client))
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cli.SetAddrs(ctx, []string{addr1, addr2})

	var mu sync.Mutex
	shards := make(map[string]bool)
	err := cli.ring.ForEachShard(ctx, func(ctx context.Context, c *redis.Client) error {
		mu.Lock()
		shards[c.Options().Addr] = true
		mu.Unlock()
		return c.Ping(ctx).Err()
	})
	if err != nil {
		t.Fatalf("Failed to ping shards with swarm mTLS: %v", err)
	}
	if !shards[addr1] || !shards[addr2] {
		t.Fatalf("Failed to get both shards, got: %v", shards)
	}
}

func TestRedisSwarmMTLSNoLeak(t *testing.T) {
	ca := nettest.NewCA(t)
	addr, done := redistest.NewTestRedisSwarmMTLS(t, ca)
	defer done()

	client := newSwarmMTLSClient(t, ca)

	t.Run("close", func(t *testing.T) {
		noleak.Check(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := redisSetGet(ctx, newRedisSwarmMTLSOptions([]string{addr}, true, client.pool, client)); err != nil {
			t.Fatalf("Failed to use swarm mTLS: %v", err)
		}
	})
}
