package main

import (
	"reflect"
	"testing"
)

func Test_secretsFlags_String(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{
			name: "test non values",
			want: "",
		},
		{
			name:   "test single value",
			values: []string{"/path/to"},
			want:   "/path/to",
		},
		{
			name:   "test more values",
			values: []string{"/path/to", "/meta/credentials"},
			want:   "/path/to,/meta/credentials",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &secretsFlags{
				values: tt.values,
			}
			if got := m.String(); got != tt.want {
				t.Errorf("secretsFlags.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_secretsFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		sf      *secretsFlags
		wantErr bool
	}{
		{
			name:    "test set no value",
			wantErr: false,
		},
		{
			name:    "test set a value on unintialized secretsFlags",
			args:    "/meta/credentials",
			wantErr: false,
		},
		{
			name:    "test set a value on intialized secretsFlags",
			sf:      &secretsFlags{},
			args:    "/meta/credentials",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.sf.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("secretsFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_secretsFlags_Get(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "test get",
			values: []string{"foo"},
			want:   []string{"foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &secretsFlags{
				values: tt.values,
			}
			if got := m.Get(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretsFlags.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
