package nitro

import (
	"crypto/tls"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/metrics/metricstest"
)

func TestParseCertificate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		want    Certificate
		wantErr bool
	}{
		{
			name:  "token and chain",
			input: "token=acm,chain=/etc/acm/chain.pem",
			want:  Certificate{TokenLabel: "acm", ChainPath: "/etc/acm/chain.pem"},
		},
		{
			name:  "with key label",
			input: "token=acm, chain=/chain.pem, key=k1",
			want:  Certificate{TokenLabel: "acm", ChainPath: "/chain.pem", KeyLabel: "k1"},
		},
		{
			name:    "missing chain",
			input:   "token=acm",
			wantErr: true,
		},
		{
			name:    "missing token",
			input:   "chain=/chain.pem",
			wantErr: true,
		},
		{
			name:    "unknown key",
			input:   "token=acm,chain=/chain.pem,foo=bar",
			wantErr: true,
		},
		{
			name:    "no value",
			input:   "token=acm,chain",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCertificate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			again, err := ParseCertificate(got.String())
			require.NoError(t, err)
			assert.Equal(t, got, again)
		})
	}
}

func TestRequireSNI(t *testing.T) {
	known := &tls.Certificate{}
	errLookup := errors.New("lookup failed")
	get := func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		switch hello.ServerName {
		case "known.example.org":
			return known, nil
		case "error.example.org":
			return nil, errLookup
		}
		return nil, nil
	}

	for _, tt := range []struct {
		name       string
		serverName string
		want       *tls.Certificate
		wantErr    error
		counter    string
	}{
		{
			name:       "known host",
			serverName: "known.example.org",
			want:       known,
		},
		{
			name:    "no sni",
			wantErr: ErrNoSNI,
			counter: "awsnitro.handshake.rejected.no_sni",
		},
		{
			name:       "unknown host",
			serverName: "unknown.example.org",
			wantErr:    ErrUnknownServerName,
			counter:    "awsnitro.handshake.rejected.unknown_host",
		},
		{
			name:       "lookup error",
			serverName: "error.example.org",
			wantErr:    errLookup,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &metricstest.MockMetrics{}
			got, err := RequireSNI(get, m)(&tls.ClientHelloInfo{ServerName: tt.serverName})
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Same(t, tt.want, got)
			if tt.counter != "" {
				m.WithCounters(func(c map[string]int64) {
					assert.Equal(t, int64(1), c[tt.counter])
				})
			}
		})
	}
}
