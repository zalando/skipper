package net

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
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

type remoteCache struct {
	client *RedisRingClient
}

func (rs *remoteCache) Get(ctx context.Context, key string) ([]byte, error) {
	res, err := rs.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return []byte(res), nil
}

func (rs *remoteCache) Delete(ctx context.Context, key string) error {
	return rs.client.Del(ctx, key)
}

func (rs *remoteCache) Put(ctx context.Context, key string, val []byte) error {
	_, err := rs.client.Set(ctx, key, val, 0)
	return err
}

func (rs *remoteCache) Close() {
	rs.client.Close()
}

type letsencrypt struct {
	manager *autocert.Manager
}

// NewLetsencrypt creates a letsencrypt handler to automatically handle CSR challenges.
//
// The cache argument can be either
//
//   - autocert.DirCache for a filesystem cache
//   - inmemoryCache for in memory cache
//   - remoteCache for redis based production cache to be shared between multiple skipper processes
func NewLetsencrypt(cache autocert.Cache, email, directoryURL string, domains []string) *letsencrypt {
	for _, s := range domains {
		if validateDomain(s) {
			domains = append(domains, s)
		}
	}

	manager := &autocert.Manager{
		Cache:      cache,
		Email:      email,
		HostPolicy: autocert.HostWhitelist(domains...),
		Prompt:     autocert.AcceptTOS,
		Client: &acme.Client{
			DirectoryURL: directoryURL,
			UserAgent:    "skipper-test",
			HTTPClient:   http.DefaultClient,
		},
	}

	return &letsencrypt{
		manager: manager,
	}
}

func (le *letsencrypt) TLSConfig() *tls.Config {
	return le.manager.TLSConfig()
}

// Listener returns a net.Listener that need to be closed on exit or
// you leak a goroutine
func (le *letsencrypt) Listener() net.Listener {
	return le.manager.Listener()
}

func (le *letsencrypt) Client() *acme.Client {
	return le.manager.Client
}

func (le *letsencrypt) Close() {
	le.Listener().Close()
}

func validateDomain(s string) bool {
	matchDomainPart, err := regexp.Compile("^[a-z0-9]+$")
	if err != nil {
		return false
	}

	i := 0
	for _, w := range strings.Split(s, ".") {
		if !matchDomainPart.MatchString(w) {
			return false
		}
		i++
	}
	return i > 1
}
