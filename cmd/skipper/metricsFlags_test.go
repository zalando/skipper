package main

import (
	"reflect"
	"testing"
)

func Test_metricsFlags_String(t *testing.T) {
	type fields struct {
		values []string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metricsFlags{
				values: tt.fields.values,
			}
			if got := m.String(); got != tt.want {
				t.Errorf("metricsFlags.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_metricsFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		values  []string
		wantErr bool
	}{
		{
			name:    "test set wrong value",
			args:    "foo",
			wantErr: true,
		},
		{
			name:    "test set codahale value",
			args:    "codahale",
			wantErr: false,
			values:  []string{"codahale"},
		},
		{
			name:    "test set prometheus value",
			args:    "prometheus",
			wantErr: false,
			values:  []string{"prometheus"},
		},
		{
			name:    "test set codahale,prometheus value",
			args:    "codahale,prometheus",
			wantErr: false,
			values:  []string{"codahale", "prometheus"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metricsFlags{
				values: tt.values,
			}
			if err := m.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("metricsFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_metricsFlags_Get(t *testing.T) {
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
			m := &metricsFlags{
				values: tt.values,
			}
			if got := m.Get(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("metricsFlags.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
