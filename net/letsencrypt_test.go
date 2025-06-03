package net

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

type inmemoryCache struct {
	m sync.Map
}

func (c *inmemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	if dat, ok := c.m.Load(key); !ok {
		return nil, fmt.Errorf("missing key %q", key)
	} else {
		if data, ok := dat.([]byte); !ok {
			return nil, fmt.Errorf("failed to convert %q to []byte", dat)
		} else {
			return data, nil
		}
	}
}

func (c *inmemoryCache) Put(ctx context.Context, key string, data []byte) error {
	c.m.Store(key, data)
	return nil
}

func (c *inmemoryCache) Delete(ctx context.Context, key string) error {
	c.m.Delete(key)
	return nil
}

func TestLetsencrypt(t *testing.T) {
	t.Log("foo")
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
		t.Logf("dir: %+v", dir)
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
