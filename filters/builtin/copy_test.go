package builtin

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestNewCopyRequestHeader(t *testing.T) {
	tests := []struct {
		name string
		want filters.Spec
	}{
		{
			name: "test copy request header constructor",
			want: &copySpec{
				typ:        CopyRequestHeader,
				filterName: requestCopyFilterName,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCopyRequestHeader(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewCopyRequestHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewCopyResponseHeader(t *testing.T) {
	tests := []struct {
		name string
		want filters.Spec
	}{
		{
			name: "test copy response header constructor",
			want: &copySpec{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCopyResponseHeader(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewCopyResponseHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_copySpec_Name(t *testing.T) {
	type fields struct {
		typ        direction
		filterName string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "test response copy filter name",
			fields: fields{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
			want: responseCopyFilterName,
		}, {
			name: "test request copy filter name",
			fields: fields{
				typ:        CopyRequestHeader,
				filterName: requestCopyFilterName,
			},
			want: requestCopyFilterName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &copySpec{
				typ:        tt.fields.typ,
				filterName: tt.fields.filterName,
			}
			if got := s.Name(); got != tt.want {
				t.Errorf("copySpec.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_copySpec_CreateFilter(t *testing.T) {
	type fields struct {
		typ        direction
		filterName string
	}
	type args struct {
		args []interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    filters.Filter
		wantErr bool
	}{
		{
			name: "test request copy filter create filter",
			fields: fields{
				typ:        CopyRequestHeader,
				filterName: requestCopyFilterName,
			},
			args: args{[]interface{}{"X-Src", "X-Dst"}},
			want: &copyFilter{
				typ: CopyRequestHeader,
				src: "X-Src",
				dst: "X-Dst",
			},
			wantErr: false,
		}, {
			name: "test response copy filter create filter",
			fields: fields{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
			args: args{[]interface{}{"X-Src", "X-Dst"}},
			want: &copyFilter{
				typ: CopyResponseHeader,
				src: "X-Src",
				dst: "X-Dst",
			},
			wantErr: false,
		}, {
			name: "test wrong args create filter",
			fields: fields{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
			args:    args{[]interface{}{5, "X-Dst"}},
			want:    nil,
			wantErr: true,
		}, {
			name: "test wrong args 2 create filter",
			fields: fields{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
			args:    args{[]interface{}{"X-Dst", 5}},
			want:    nil,
			wantErr: true,
		}, {
			name: "test wrong args 3 create filter",
			fields: fields{
				typ:        CopyResponseHeader,
				filterName: responseCopyFilterName,
			},
			args:    args{[]interface{}{"X-foo"}},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &copySpec{
				typ:        tt.fields.typ,
				filterName: tt.fields.filterName,
			}
			got, err := s.CreateFilter(tt.args.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("copySpec.CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("copySpec.CreateFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func buildfilterSetRequestContext() filters.FilterContext {
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
		typ direction
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
				typ: CopyRequestHeader,
				src: "X-Src",
				dst: "X-Dst",
			},
			args: args{
				ctx: buildfilterSetRequestContext(),
			},
			expect: "header src content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := copyFilter{
				typ: tt.fields.typ,
				src: tt.fields.src,
				dst: tt.fields.dst,
			}
			f.Request(tt.args.ctx)
			got := tt.args.ctx.Request().Header.Get(f.dst)
			if got != tt.expect {
				t.Errorf("'%s' expected '%s'", got, tt.expect)
			}
		})
	}
}

func Test_copyFilter_Response(t *testing.T) {
	type fields struct {
		typ direction
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
				typ: CopyResponseHeader,
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
			f := copyFilter{
				typ: tt.fields.typ,
				src: tt.fields.src,
				dst: tt.fields.dst,
			}
			f.Response(tt.args.ctx)
			got := tt.args.ctx.Response().Header.Get(f.dst)
			if got != tt.expect {
				t.Errorf("'%s' expected '%s'", got, tt.expect)
			}

		})
	}
}
