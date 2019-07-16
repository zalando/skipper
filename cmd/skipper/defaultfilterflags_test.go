package main

import (
	"reflect"
	"testing"

	"github.com/zalando/skipper/eskip"
)

func Test_defaultFiltersFlags_String(t *testing.T) {
	tests := []struct {
		name    string
		filters []*eskip.Filter
		want    string
	}{
		{
			name:    "test string",
			filters: []*eskip.Filter{},
			want:    "<default filters>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpf := &defaultFiltersFlags{
				filters: tt.filters,
			}
			if got := dpf.String(); got != tt.want {
				t.Errorf("defaultFiltersFlags.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_defaultFiltersFlags_Set(t *testing.T) {
	oneFilter, _ := eskip.ParseFilters(`tee("https://www.zalando.de/")`)
	manyFilters, _ := eskip.ParseFilters(`ratelimit(5, "10s") -> tee("https://www.zalando.de/")`)
	tests := []struct {
		name    string
		args    string
		want    []*eskip.Filter
		wantErr bool
	}{
		{
			name:    "test no filter",
			args:    "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "test one filter",
			args:    `tee("https://www.zalando.de/")`,
			want:    oneFilter,
			wantErr: false,
		},
		{
			name:    "test many filters",
			args:    `ratelimit(5, "10s") -> tee("https://www.zalando.de/")`,
			want:    manyFilters,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpf := &defaultFiltersFlags{}
			if err := dpf.Set(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("defaultFiltersFlags.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(tt.want) != len(dpf.filters) {
					t.Errorf("defaultFiltersFlags size missmatch got %d want %d", len(dpf.filters), len(tt.want))
				}
			}
		})
	}
}
