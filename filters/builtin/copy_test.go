package builtin

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func Test_copySpec_CreateFilter(t *testing.T) {
	tests := []struct {
		name    string
		newSpec func() filters.Spec
		args    []any
		want    filters.Filter
		wantErr bool
	}{
		{
			name:    "test request copy filter create filter",
			newSpec: NewCopyRequestHeader,
			args:    []any{"X-Src", "X-Dst"},
			want: &headerFilter{
				typ:   copyRequestHeader,
				key:   "X-Src",
				value: "X-Dst",
			},
			wantErr: false,
		}, {
			name:    "test response copy filter create filter",
			newSpec: NewCopyResponseHeader,
			args:    []any{"X-Src", "X-Dst"},
			want: &headerFilter{
				typ:   copyResponseHeader,
				key:   "X-Src",
				value: "X-Dst",
			},
			wantErr: false,
		}, {
			name:    "test wrong args create filter",
			newSpec: NewCopyResponseHeader,
			args:    []any{5, "X-Dst"},
			wantErr: true,
		}, {
			name:    "test wrong args 2 create filter",
			newSpec: NewCopyResponseHeader,
			args:    []any{"X-Dst", 5},
			wantErr: true,
		}, {
			name:    "test wrong args 3 create filter",
			newSpec: NewCopyResponseHeader,
			args:    []any{"X-foo"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.newSpec()
			got, err := s.CreateFilter(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("copySpec.CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("copySpec.CreateFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func buildfilterRequestContext() filters.FilterContext {
	r, _ := http.NewRequest("GET", "http://example.org/api/v3", nil)
	r.Header.Add("X-Src", "header src content")
	return &filtertest.Context{FRequest: r}
}

func buildfilterResponseContext() filters.FilterContext {
	r := &http.Response{Header: make(http.Header)}
	r.Header.Add("X-Src", "header src content")
	return &filtertest.Context{FResponse: r}
}

func Test_copyFilter_Request(t *testing.T) {
	type fields struct {
		src string
		dst string
	}
	type args struct {
		ctx filters.FilterContext
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		expect string
	}{
		{
			name: "request copy header",
			fields: fields{
				src: "X-Src",
				dst: "X-Dst",
			},
			args: args{
				ctx: buildfilterRequestContext(),
			},
			expect: "header src content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := NewCopyRequestHeader().CreateFilter([]any{
				tt.fields.src,
				tt.fields.dst,
			})

			f.Request(tt.args.ctx)
			got := tt.args.ctx.Request().Header.Get(tt.fields.dst)
			if got != tt.expect {
				t.Errorf("'%s' expected '%s'", got, tt.expect)
			}
		})
	}
}

func Test_copyFilter_Response(t *testing.T) {
	type fields struct {
		src string
		dst string
	}
	type args struct {
		ctx filters.FilterContext
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		expect string
	}{
		{
			name: "response copy header",
			fields: fields{
				src: "X-Src",
				dst: "X-Dst",
			},
			args: args{
				ctx: buildfilterResponseContext(),
			},
			expect: "header src content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := NewCopyResponseHeader().CreateFilter([]any{
				tt.fields.src,
				tt.fields.dst,
			})

			f.Response(tt.args.ctx)
			got := tt.args.ctx.Response().Header.Get(tt.fields.dst)
			if got != tt.expect {
				t.Errorf("'%s' expected '%s'", got, tt.expect)
			}

		})
	}
}
