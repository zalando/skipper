package builtin

import (
	"net/http"
	"reflect"
	"slices"
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

func TestCopyHeaderMultipleValues(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		f, err := NewCopyRequestHeader().CreateFilter([]any{"X-Forwarded-For", "X-Real-Forwarded-For"})
		if err != nil {
			t.Fatal(err)
		}

		req, err := http.NewRequest("GET", "https://example.org/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header["X-Forwarded-For"] = []string{"10.0.1.1", "10.0.2.1"}

		f.Request(&filtertest.Context{FRequest: req})

		want := []string{"10.0.1.1", "10.0.2.1"}
		if got := req.Header.Values("X-Real-Forwarded-For"); !slices.Equal(got, want) {
			t.Errorf("failed to copy all request header values, got: %q, want: %q", got, want)
		}
		if got := req.Header.Values("X-Forwarded-For"); !slices.Equal(got, want) {
			t.Errorf("source request header values were changed, got: %q, want: %q", got, want)
		}
	})

	t.Run("response", func(t *testing.T) {
		f, err := NewCopyResponseHeader().CreateFilter([]any{"Set-Cookie", "X-Backend-Set-Cookie"})
		if err != nil {
			t.Fatal(err)
		}

		rsp := &http.Response{Header: http.Header{"Set-Cookie": []string{
			"a=1; Domain=example.org",
			"b=2; Domain=example.org",
		}}}

		f.Response(&filtertest.Context{FResponse: rsp})

		want := []string{"a=1; Domain=example.org", "b=2; Domain=example.org"}
		if got := rsp.Header.Values("X-Backend-Set-Cookie"); !slices.Equal(got, want) {
			t.Errorf("failed to copy all response header values, got: %q, want: %q", got, want)
		}
		if got := rsp.Header.Values("Set-Cookie"); !slices.Equal(got, want) {
			t.Errorf("source response header values were changed, got: %q, want: %q", got, want)
		}
	})

	t.Run("response onto itself", func(t *testing.T) {
		f, err := NewCopyResponseHeader().CreateFilter([]any{"Set-Cookie", "Set-Cookie"})
		if err != nil {
			t.Fatal(err)
		}

		want := []string{"a=1; Domain=example.org", "b=2; Domain=example.org"}
		rsp := &http.Response{Header: http.Header{"Set-Cookie": append([]string(nil), want...)}}

		f.Response(&filtertest.Context{FResponse: rsp})

		if got := rsp.Header.Values("Set-Cookie"); !slices.Equal(got, want) {
			t.Errorf("copying a header onto itself dropped values, got: %q, want: %q", got, want)
		}
	})
}
