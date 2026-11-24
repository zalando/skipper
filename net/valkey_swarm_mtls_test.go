package net

import (
	"context"
	"crypto/x509"
	"regexp"
	"testing"
	"time"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/zalando/skipper/net/nettest"
	"github.com/zalando/skipper/net/valkeytest"
)

type swarmMTLSClient struct {
	pool     *x509.CertPool
	certFile string
	keyFile  string
}

func newSwarmMTLSClient(t *testing.T, ca *nettest.CA) swarmMTLSClient {
	t.Helper()
	certPEM, keyPEM := ca.ClientCert(t)
	certFile, keyFile := nettest.WriteCertKey(t, certPEM, keyPEM)
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	return swarmMTLSClient{pool: pool, certFile: certFile, keyFile: keyFile}
}

func newValkeySwarmMTLSOptions(addrs []string, enable bool, pool *x509.CertPool, client swarmMTLSClient) *ValkeyOptions {
	return &ValkeyOptions{
		Addrs:            addrs,
		EnableMTLS:       enable,
		CA:               pool,
		ClientCertFile:   client.certFile,
		ClientKeyFile:    client.keyFile,
		DialTimeout:      time.Second,
		ConnWriteTimeout: time.Second,
	}
}

func TestValkeySwarmMTLS(t *testing.T) {
	ca := nettest.NewCA(t)
	otherCA := nettest.NewCA(t)
	addr, done := valkeytest.NewTestValkeySwarmMTLS(t, ca)
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

			err := valkeySetGet(ctx, newValkeySwarmMTLSOptions([]string{addr}, tt.enable, tt.pool, tt.client))
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

func valkeySetGet(ctx context.Context, opt *ValkeyOptions) error {
	cli, err := NewValkeyRingClient(opt)
	if err != nil {
		return err
	}
	defer cli.Close()

	if _, err := cli.Set(ctx, "k", "v"); err != nil {
		return err
	}
	_, err = cli.Get(ctx, "k")
	return err
}

func TestValkeySwarmMTLSSetAddr(t *testing.T) {
	ca := nettest.NewCA(t)
	addr1, done1 := valkeytest.NewTestValkeySwarmMTLS(t, ca)
	defer done1()
	addr2, done2 := valkeytest.NewTestValkeySwarmMTLS(t, ca)
	defer done2()

	client := newSwarmMTLSClient(t, ca)
	cli, err := NewValkeyRingClient(newValkeySwarmMTLSOptions([]string{addr1}, true, client.pool, client))
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cli.SetAddrs(ctx, []string{addr1, addr2}); err != nil {
		t.Fatalf("Failed to set addresses: %v", err)
	}
	for _, addr := range []string{addr1, addr2} {
		if err := cli.Ping(ctx, addr); err != nil {
			t.Errorf("Failed to ping %s with swarm mTLS: %v", addr, err)
		}
	}
}

func TestValkeySwarmMTLSNoLeak(t *testing.T) {
	ca := nettest.NewCA(t)
	addr, done := valkeytest.NewTestValkeySwarmMTLS(t, ca)
	defer done()

	client := newSwarmMTLSClient(t, ca)

	t.Run("close", func(t *testing.T) {
		noleak.Check(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := valkeySetGet(ctx, newValkeySwarmMTLSOptions([]string{addr}, true, client.pool, client)); err != nil {
			t.Fatalf("Failed to use swarm mTLS: %v", err)
		}
	})
}
