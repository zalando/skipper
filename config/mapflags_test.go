package config

import (
	"testing"
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
			}
		})
	}
}
