package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

func Test_mapFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		values  map[string]string
		wantErr bool
	}{
		{
			name:    "test set invalid value",
			args:    "foo",
			wantErr: true,
		},
		{
			name:    "test set invalid value",
			args:    "foo=",
			wantErr: true,
		},
		{
			name:    "test set invalid value",
			args:    "=bar",
			wantErr: true,
		},
		{
			name:    "test set foo=bar value",
			args:    "foo=bar",
			wantErr: false,
			values:  map[string]string{"foo": "bar"},
		},
		{
			name:    "test set foo=bar=baz value",
			args:    "foo=bar=baz",
			wantErr: false,
			values:  map[string]string{"foo": "bar=baz"},
		},
		{
			name:    "test set ' foo = bar ' value",
			args:    " foo = bar ",
			wantErr: false,
			values:  map[string]string{"foo": "bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMapFlags()

			if err := m.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			} else if err == nil && len(m.values) != len(tt.values) {
				t.Errorf("parse failed, got: %v, expected: %v", m.values, tt.values)
			} else if err == nil {
				for k, v := range m.values {
					if v != tt.values[k] {
						t.Errorf(
							"parse failed for %s, got: %s, expected: %s",
							k, v, tt.values[k],
						)
					}
				}
				if len(m.values) > 0 {
					if s := m.String(); s == "" {
						t.Errorf("Failed to get string value for non empty mapFlag: %v", m.values)
					}
				}
			}
		})
	}
}

func Test_mapFlags_Set_nil(t *testing.T) {
	var m *mapFlags
	if err := m.Set("foo"); err != nil || m != nil {
		t.Error("Failed to get nil error or lazy init if m is nil")
	}
}

func Test_mapFlags_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		wantErr bool
		want    map[string]string
	}{
		{
			name: "test mapflags",
			yml: `---
key1: value
key2: 100
key3: k3=10s`,
			wantErr: false,
			want: map[string]string{
				"key1": "value",
				"key2": "100",
				"key3": "k3=10s",
			},
		},
		{
			name: "test mapFlags with yaml error",
			yml: `---
foo=bar`,
			wantErr: true,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &mapFlags{}

			if err := yaml.Unmarshal([]byte(tt.yml), mf); (err != nil) != tt.wantErr {
				t.Errorf("mapFlags.UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(mf.values) != len(tt.want) {
					t.Errorf("Failed to have mapFlags created: %d != %d", len(mf.values), len(tt.want))
				}

				if cmp.Diff(mf.values, tt.want) != "" {
					t.Errorf("mapFlags.UnmarshalYAML() got %v, want %v", mf.values, tt.want)
				}
			}
		})
	}
}
