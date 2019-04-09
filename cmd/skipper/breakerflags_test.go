package main

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/circuit"
)

func Test_breakerFlags_String(t *testing.T) {
	tests := []struct {
		name string
		b    *breakerFlags
		want string
	}{
		{
			name: "test consecutive breaker",
			b: &breakerFlags{
				circuit.BreakerSettings{
					Type:             circuit.ConsecutiveFailures,
					Host:             "example.com",
					Window:           10,
					Timeout:          3 * time.Second,
					HalfOpenRequests: 3,
					IdleTTL:          5 * time.Second,
				},
			},
			want: "type=consecutive,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.b.String(); got != tt.want {
				t.Errorf("breakerFlags.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_breakerFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
		want    circuit.BreakerSettings
	}{
		{
			name:    "test breaker settings",
			args:    "type=consecutive,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
			wantErr: false,
			want: circuit.BreakerSettings{
				Type:             circuit.ConsecutiveFailures,
				Host:             "example.com",
				Timeout:          3 * time.Second,
				HalfOpenRequests: 3,
				IdleTTL:          5 * time.Second,
			},
		},
		{
			name:    "test breaker settings with window",
			args:    "type=consecutive,window=4,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
			wantErr: false,
			want: circuit.BreakerSettings{
				Type:             circuit.ConsecutiveFailures,
				Host:             "example.com",
				Timeout:          3 * time.Second,
				HalfOpenRequests: 3,
				IdleTTL:          5 * time.Second,
				Window:           4,
			},
		},
		{
			name:    "test breaker settings with wrong window",
			args:    "type=consecutive,window=4s,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
			wantErr: true,
		},
		{
			name:    "test breaker settings failurerate",
			args:    "type=rate,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
			wantErr: false,
			want: circuit.BreakerSettings{
				Type:             circuit.FailureRate,
				Host:             "example.com",
				Timeout:          3 * time.Second,
				HalfOpenRequests: 3,
				IdleTTL:          5 * time.Second,
			},
		},
		{
			name:    "test breaker settings disabled",
			args:    "type=disabled,host=example.com,timeout=3s,half-open-requests=3,idle-ttl=5s",
			wantErr: false,
			want: circuit.BreakerSettings{
				Type:             circuit.BreakerDisabled,
				Host:             "example.com",
				Timeout:          3 * time.Second,
				HalfOpenRequests: 3,
				IdleTTL:          5 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &breakerFlags{}

			if err := bp.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("breakerFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				b := *bp
				if len(b) != 1 {
					t.Errorf("Failed to have breaker created: %d != 1", len(b))
				}

				bs := b[0]
				if bs.Type != tt.want.Type || bs.Host != tt.want.Host || bs.Window != tt.want.Window || bs.Failures != tt.want.Failures || bs.Timeout != tt.want.Timeout || bs.HalfOpenRequests != tt.want.HalfOpenRequests || bs.IdleTTL != tt.want.IdleTTL {
					t.Errorf("breakerFlags.Set() got v, want v, %v", cmp.Diff(bs, tt.want))
				}
			}

		})
	}
}
