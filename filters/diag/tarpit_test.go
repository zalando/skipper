package diag

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestTarpit(t *testing.T) {
	for _, tt := range []struct {
		name          string
		args          []any
		status        int
		clientTimeout time.Duration
		want          error
	}{
		{
			name: "test no args return error",
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "test wrong arg return error",
			args: []any{"no-time-duration"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "test no string arg return error",
			args: []any{0x0a},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name: "test wrong number of args return error",
			args: []any{"10s", "10ms"},
			want: filters.ErrInvalidFilterParameters,
		},
		{
			name:          "test 10ms and 1s client timeout",
			args:          []any{"10ms"},
			clientTimeout: time.Second,
			want:          nil,
		},
		{
			name:          "test 1s and 1s client timeout",
			args:          []any{"1s"},
			clientTimeout: time.Second,
			want:          nil,
		},
		{
			name:          "test 1s and 100ms client timeout",
			args:          []any{"100ms"},
			clientTimeout: time.Second,
			want:          nil,
		},
		{
			name:          "test 1s and 3s client timeout",
			args:          []any{"1s"},
			clientTimeout: 3 * time.Second,
			want:          nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {

			}))
			defer backend.Close()

			spec := NewTarpit()
			_, err := spec.CreateFilter(tt.args)
			switch err {
			case tt.want:
				// ok
				if err != nil {
					return
				}
			default:
				t.Fatal(err)
			}

			fr := filters.Registry{}
			fr.Register(spec)
			sargs := make([]string, 0, len(tt.args))
			for _, e := range tt.args {
				sargs = append(sargs, e.(string))
			}
			doc := fmt.Sprintf(`r: * -> tarpit("%s") -> "%s";`, strings.Join(sargs, ","), backend.URL)
			r := eskip.MustParse(doc)
			p := proxytest.New(fr, r...)
			defer p.Close()

			N := 1
			for range N {
				ctx, done := context.WithTimeout(context.Background(), tt.clientTimeout)
				defer done()
				req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				rsp, err := p.Client().Do(req)
				if err != nil {
					t.Fatalf("Failed to get response: %v", err)
				}

				if rsp.StatusCode != 200 {
					t.Fatalf("Failed to get status code 200 got: %d", rsp.StatusCode)
				}
			}
		})
	}
}
