package main

import (
	"reflect"
	"testing"
)

func Test_parseHistogramBuckets(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    string
		want    []float64
		wantErr bool
	}{
		{
			name:    "test parse 1",
			args:    "1",
			want:    []float64{1},
			wantErr: false,
		},
		{
			name:    "test parse 1,1.33,1.5,1.66,2",
			args:    "1,1.33,1.5,1.66,2",
			want:    []float64{1, 1.33, 1.5, 1.66, 2},
			wantErr: false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHistogramBuckets(tt.args)
			if !reflect.DeepEqual(got, tt.want) || (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Errorf("Failed to parse histogram buckets: Want %v, got %v, err %v", tt.want, got, err)
			}
		})
	}

}
