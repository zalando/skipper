package net

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/net/valkeytest"
	"golang.org/x/crypto/acme/autocert"
)

func TestRemoteCache(t *testing.T) {
	t.Logf("create valkey..")
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	if valkeyAddr == "" {
		t.Fatal("Failed to create valkey 1")
	}

	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()
	if valkeyAddr2 == "" {
		t.Fatal("Failed to create valkey 2")
	}

	client, err := NewValkeyRingClient(&ValkeyOptions{
		Addrs: []string{valkeyAddr, valkeyAddr2},
	})
	if err != nil {
		t.Fatalf("Failed to create remote cahce client: %v", err)
	}

	rc := RemoteCache{
		Client: client,
	}
	defer rc.Close()

	if v, err := rc.Get(context.Background(), "not-exist"); err != nil {
		if err != autocert.ErrCacheMiss {
			t.Fatalf("Failed to get cache miss error on not existing key: %v err: %v", v, err)
		}
	} else {
		t.Fatalf("Want error %q but got %q", autocert.ErrCacheMiss, v)
	}

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
	rc := &InmemoryCache{}

	if _, err := rc.Get(context.Background(), "foo"); err != autocert.ErrCacheMiss {
		t.Fatal(`Failed can not get cache miss on empty cache`)
	}

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

	if err := rc.Put(context.Background(), "foo2", []byte("ü")); err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	if v, err := rc.Get(context.Background(), "foo2"); err != nil {
		t.Fatalf("Failed to get: %v", err)
	} else {
		t.Logf("%T %v %s", v, v, v)
	}

}

func TestLetsencrypt(t *testing.T) {
	invalidDomain := "s_.example.org"
	require.False(t, validateDomain(invalidDomain), "Failed to validate invalid domain %q", invalidDomain)

	validDomain := "example.org"
	require.True(t, validateDomain(validDomain), "Failed to validate valid domain %q", validDomain)

	wildcardDomain := "*.example.org"
	require.True(t, validateDomain(wildcardDomain), "Failed to validate valid wildcard domain %q", wildcardDomain)

	invalidDoubleWildcardDomain := "*.*.example.org"
	require.False(t, validateDomain(invalidDoubleWildcardDomain), "Failed to validate invalid double wildcard domain %q", invalidDoubleWildcardDomain)

	le := NewLetsencrypt(&InmemoryCache{}, "skipper@example.org", "https://acme-staging-v02.api.letsencrypt.org/directory", "skipper-test TestLetsencrypt", []string{validDomain, wildcardDomain})
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

	require.NotNil(t, le.Client(), "client should not be nil")
	require.NotNil(t, le.TLSConfig(), "TLSConfig should not be nil")
	require.NotNil(t, le.Handler(nil), "http.Handler should not be nil")

	li := le.Listener()
	defer li.Close()
	t.Logf("listener %v", li.Addr())
}

func TestLetsencryptWildcardHostPolicy(t *testing.T) {
	le := NewLetsencrypt(
		&InmemoryCache{},
		"skipper@example.org",
		"https://acme-staging-v02.api.letsencrypt.org/directory",
		"skipper-test TestLetsencryptWildcardHostPolicy",
		[]string{"*.example.org", "example.com"},
	)
	defer le.Close()

	for _, tt := range []struct {
		name    string
		host    string
		allowed bool
	}{
		{"wildcard matches single-label subdomain", "foo.example.org", true},
		{"wildcard does not match wrong bare domain", "example.org", false},
		{"wildcard does not match multi-label subdomain", "a.b.example.org", false},
		{"exact domain matches", "example.com", true},
		{"exact domain does not match subdomain", "foo.example.com", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := le.manager.HostPolicy(context.Background(), tt.host)
			if tt.allowed {
				require.NoError(t, err, "host %q should be allowed", tt.host)
			} else {
				require.Error(t, err, "host %q should be rejected", tt.host)
			}
		})
	}
}
