package main

import (
	"testing"
	"time"

	"github.com/zalando/skipper/ratelimit"
)

func Test_ratelimitFlags_String(t *testing.T) {
	tests := []struct {
		name string
		r    *ratelimitFlags
		want string
	}{
		{
			name: "test ratelimit",
			r: &ratelimitFlags{
				ratelimit.Settings{
					Type:       ratelimit.ServiceRatelimit,
					MaxHits:    10,
					TimeWindow: 10 * time.Second,
				},
			},
			want: "ratelimit(type=service,max-hits=10,time-window=10s)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("ratelimitFlags.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ratelimitFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
		want    ratelimit.Settings
	}{
		{
			name:    "test ratelimit",
			args:    "type=service,max-hits=100,time-window=10s",
			wantErr: false,
			want: ratelimit.Settings{
				Type:       ratelimit.ServiceRatelimit,
				MaxHits:    100,
				TimeWindow: 10 * time.Second,
			},
		},
		{
			name:    "test client ratelimit",
			args:    "type=client,max-hits=50,time-window=2m",
			wantErr: false,
			want: ratelimit.Settings{
				Type:       ratelimit.ClientRatelimit,
				MaxHits:    50,
				TimeWindow: 2 * time.Minute,
			},
		},
		{
			name:    "test disabled ratelimit",
			args:    "type=disabled,max-hits=50,time-window=2m",
			wantErr: false,
			want: ratelimit.Settings{
				Type:       ratelimit.DisableRatelimit,
				MaxHits:    50,
				TimeWindow: 2 * time.Minute,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &ratelimitFlags{}

			if err := rp.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("ratelimitFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				r := *rp
				if len(r) != 1 {
					t.Errorf("Failed to have ratelimit created: %d != 1", len(r))
				}

				rs := r[0]
				if rs.Type != tt.want.Type || rs.MaxHits != tt.want.MaxHits || rs.TimeWindow != tt.want.TimeWindow {
					t.Errorf("ratelimitFlags.Set() got %v, want %v", rs, tt.want)
				}
			}
		})
	}
}
