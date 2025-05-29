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

type InmemoryCache struct {
	m sync.Map
}

func (ic *InmemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	if dat, ok := ic.m.Load(key); !ok {
		return nil, fmt.Errorf("missing key %q", key)
	} else {
		if data, ok := dat.([]byte); !ok {
			return nil, fmt.Errorf("failed to convert %q to []byte", dat)
		} else {
			return data, nil
		}
	}
}

func (ic *InmemoryCache) Put(ctx context.Context, key string, data []byte) error {
	ic.m.Store(key, data)
	return nil
}

func (ic *InmemoryCache) Delete(ctx context.Context, key string) error {
	ic.m.Delete(key)
	return nil
}

type RemoteCache struct {
	Client *RedisRingClient
}

func (rc *RemoteCache) Get(ctx context.Context, key string) ([]byte, error) {
	res, err := rc.Client.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return []byte(res), nil
}

func (rc *RemoteCache) Delete(ctx context.Context, key string) error {
	return rc.Client.Del(ctx, key)
}

func (rc *RemoteCache) Put(ctx context.Context, key string, val []byte) error {
	_, err := rc.Client.Set(ctx, key, val, 0)
	return err
}

func (rc *RemoteCache) Close() {
	rc.Client.Close()
}

type Letsencrypt struct {
	manager *autocert.Manager
}

// NewLetsencrypt creates a letsencrypt handler to automatically handle CSR challenges.
//
// The cache argument can be either
//
//   - autocert.DirCache for a filesystem cache
//   - inmemoryCache for in memory cache
//   - remoteCache for redis based production cache to be shared between multiple skipper processes
func NewLetsencrypt(cache autocert.Cache, email, directoryURL, userAgent string, proposedDomains []string) *Letsencrypt {
	domains := make([]string, 0, len(proposedDomains))
	for _, s := range proposedDomains {
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
			UserAgent:    userAgent,
			HTTPClient:   http.DefaultClient,
		},
	}

	return &Letsencrypt{
		manager: manager,
	}
}

func (le *Letsencrypt) Handler(fallback http.Handler) http.Handler {
	return le.manager.HTTPHandler(fallback)
}

func (le *Letsencrypt) TLSConfig() *tls.Config {
	return le.manager.TLSConfig()
}

// Listener returns a net.Listener that need to be closed on exit or
// you leak a goroutine
func (le *Letsencrypt) Listener() net.Listener {
	return le.manager.Listener()
}

func (le *Letsencrypt) Client() *acme.Client {
	return le.manager.Client
}

func (le *Letsencrypt) Close() {
	le.Listener().Close()
}

var domainRegex = regexp.MustCompile("^[a-z0-9]+$")

func validateDomain(s string) bool {
	i := 0
	for w := range strings.SplitSeq(s, ".") {
		if !domainRegex.MatchString(w) {
			return false
		}
		i++
	}
	return i > 1
}
