package main

import (
	"testing"
)

func Test_secretsFlags_Set(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		sf      *listFlag
		wantErr bool
	}{
		{
			name:    "test set no value",
			wantErr: false,
		},
		{
			name:    "test set a value on uninitialized secretsFlags",
			args:    "/meta/credentials",
			wantErr: false,
		},
		{
			name:    "test set a value on initialized secretsFlags",
			sf:      commaListFlag(),
			args:    "/meta/credentials",
			wantErr: false,
		},
		{
			name:    "test set multiple values on initialized secretsFlags",
			sf:      commaListFlag(),
			args:    "/meta/credentials,/bar",
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
