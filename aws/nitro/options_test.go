package nitro

import (
	"testing"
	"time"
)

func TestOptionsValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		opts    *Options
		wantErr bool
		want    Options
	}{
		{
			name:    "nil options",
			opts:    nil,
			wantErr: true,
		},
		{
			name:    "empty endpoint",
			opts:    &Options{},
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			opts:    &Options{Endpoint: "tcp://127.0.0.1:8000"},
			wantErr: true,
		},
		{
			name:    "invalid vsock missing port",
			opts:    &Options{Endpoint: "vsock://16"},
			wantErr: true,
		},
		{
			name:    "invalid vsock cid",
			opts:    &Options{Endpoint: "vsock://abc:8000"},
			wantErr: true,
		},
		{
			name:    "invalid vsock port zero",
			opts:    &Options{Endpoint: "vsock://16:0"},
			wantErr: true,
		},
		{
			name:    "invalid unix missing path",
			opts:    &Options{Endpoint: "unix://"},
			wantErr: true,
		},
		{
			name: "negative refresh interval",
			opts: &Options{
				Endpoint:        "vsock://16:8000",
				RefreshInterval: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			opts: &Options{
				Endpoint: "vsock://16:8000",
				Timeout:  -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative max sessions",
			opts: &Options{
				Endpoint:    "vsock://16:8000",
				MaxSessions: -1,
			},
			wantErr: true,
		},
		{
			name: "valid vsock defaults applied",
			opts: &Options{Endpoint: "vsock://16:8000"},
			want: Options{
				Endpoint:        "vsock://16:8000",
				RefreshInterval: DefaultRefreshInterval,
				Timeout:         DefaultTimeout,
				MaxSessions:     DefaultMaxSessions,
			},
		},
		{
			name: "valid unix custom values",
			opts: &Options{
				Endpoint:        "unix:///run/p11-kit/pkcs11",
				TokenLabel:      "acm",
				RefreshInterval: time.Minute,
				Timeout:         2 * time.Second,
				MaxSessions:     4,
			},
			want: Options{
				Endpoint:        "unix:///run/p11-kit/pkcs11",
				TokenLabel:      "acm",
				RefreshInterval: time.Minute,
				Timeout:         2 * time.Second,
				MaxSessions:     4,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *tt.opts != tt.want {
				t.Fatalf("got %+v, want %+v", *tt.opts, tt.want)
			}
		})
	}
}
