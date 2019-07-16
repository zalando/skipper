package main

import (
	"testing"
)

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
			f := commaListFlag("codahale", "prometheus")
			if err := f.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			} else if err == nil && len(f.values) != len(tt.values) {
				t.Errorf("parse failed, got: %v, expected: %v", f.values, tt.values)
			} else if err == nil {
				for i, v := range f.values {
					if v != tt.values[i] {
						t.Errorf(
							"parse failed at %d, got: %s, expected: %s",
							i, v, tt.values[i],
						)
					}
				}
			}
		})
	}
}
