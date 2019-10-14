package main

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/google/go-cmp/cmp"
)

func Test_pluginFlag_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
		want    [][]string
	}{
		{
			name:    "test plugin flag",
			args:    "geoip,db=/Users/test/test.mmdb",
			wantErr: false,
			want: [][]string{
				{"geoip", "db=/Users/test/test.mmdb"},
			},
		},
		{
			name:    "test plugin flag with two plugins",
			args:    "geoip,db=/Users/test/test.mmdb inet,timeout=1000,delay=2000",
			wantErr: false,
			want: [][]string{
				{"geoip", "db=/Users/test/test.mmdb"},
				{"inet", "timeout=1000", "delay=2000"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := newPluginFlag()

			if err := pf.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("pluginFlag.Set() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if cmp.Equal(pf.values, tt.want) == false {
					t.Errorf("pluginFlag.Set() got v, want v, %v", cmp.Diff(pf.values, tt.want))
				}
			}
		})
	}
}

func Test_pluginFlag_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		wantErr bool
		want    [][]string
	}{
		{
			name: "test plugin flag",
			yml: `geoip:
- db=/Users/test/test.mmdb`,
			wantErr: false,
			want: [][]string{
				{"geoip", "db=/Users/test/test.mmdb"},
			},
		},
		{
			name: "test plugin flag with two plugins",
			yml: `geoip:
- db=/Users/test/test.mmdb
inet:
- timeout=1000
- delay=2000`,
			wantErr: false,
			want: [][]string{
				{"geoip", "db=/Users/test/test.mmdb"},
				{"inet", "timeout=1000", "delay=2000"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := newPluginFlag()

			if err := yaml.Unmarshal([]byte(tt.yml), pf); (err != nil) != tt.wantErr {
				t.Errorf("pluginFlag.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if cmp.Equal(pf.values, tt.want) == false {
					t.Errorf("pluginFlag.UnmarshalYAML() got v, want v, %v", cmp.Diff(pf.values, tt.want))
				}
			}
		})
	}
}
