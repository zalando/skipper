package main

import (
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/google/go-cmp/cmp"
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
				Type:          ratelimit.ServiceRatelimit,
				MaxHits:       100,
				TimeWindow:    10 * time.Second,
				Group:         "",
				CleanInterval: 10 * time.Second * 10,
			},
		},
		{
			name:    "test client ratelimit",
			args:    "type=client,max-hits=50,time-window=2m",
			wantErr: false,
			want: ratelimit.Settings{
				Type:          ratelimit.ClientRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name:    "test disabled ratelimit",
			args:    "type=disabled,max-hits=50,time-window=2m",
			wantErr: false,
			want: ratelimit.Settings{
				Type:          ratelimit.DisableRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name:    "test invalid type",
			args:    "type=invalid,max-hits=50,time-window=2m",
			wantErr: true,
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

				if cmp.Diff(r[0], tt.want) != "" {
					t.Errorf("ratelimitFlags.Set() got %v, want %v", r[0], tt.want)
				}
			}
		})
	}
}

func Test_ratelimitFlags_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		wantErr bool
		want    ratelimit.Settings
	}{
		{
			name: "test ratelimit",
			yml: `type: service
max-hits: 100
time-window: 10s`,
			wantErr: false,
			want: ratelimit.Settings{
				Type:          ratelimit.ServiceRatelimit,
				MaxHits:       100,
				TimeWindow:    10 * time.Second,
				Group:         "",
				CleanInterval: 10 * time.Second * 10,
			},
		},
		{
			name: "test client ratelimit",
			yml: `type: client
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: ratelimit.Settings{
				Type:          ratelimit.ClientRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test disabled ratelimit",
			yml: `type: disabled
max-hits: 50
time-window: 2m`,
			wantErr: false,
			want: ratelimit.Settings{
				Type:          ratelimit.DisableRatelimit,
				MaxHits:       50,
				TimeWindow:    2 * time.Minute,
				Group:         "",
				CleanInterval: 2 * time.Minute * 10,
			},
		},
		{
			name: "test invalid type",
			yml: `type: invalid
max-hits: 50
time-window: 2m`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &ratelimitFlags{}

			if err := yaml.Unmarshal([]byte(tt.yml), rp); (err != nil) != tt.wantErr {
				t.Errorf("ratelimitFlags.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				r := *rp
				if len(r) != 1 {
					t.Errorf("Failed to have ratelimit created: %d != 1", len(r))
				}

				if cmp.Diff(r[0], tt.want) != "" {
					t.Errorf("ratelimitFlags.UnmarshalYAML() got %v, want %v", r[0], tt.want)
				}
			}
		})
	}
}
