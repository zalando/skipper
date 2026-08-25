package builtin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_originMarkerSpec_CreateFilter(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		wantErr bool
		want    *OriginMarker
	}{
		{
			name:    "no args",
			wantErr: true,
		},
		{
			name:    "time not formatted correctly",
			args:    []any{"origin", "id", "wrong time"},
			wantErr: true,
		},
		{
			name:    "parse time",
			args:    []any{"origin", "id", time0.Format(time.RFC3339)},
			wantErr: false,
			want:    &OriginMarker{"origin", "id", time0},
		},
		{
			name:    "pass time",
			args:    []any{"origin", "id", time0},
			wantErr: false,
			want:    &OriginMarker{"origin", "id", time0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewOriginMarkerSpec().CreateFilter(tt.args)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, f)
			}
		})
	}
}
