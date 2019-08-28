package scheduler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/scheduler"
)

func TestNewLIFO(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []interface{}
		schedFunc  func() filters.Spec
		wantName   string
		wantKey    string
		wantErr    bool
		wantConfig scheduler.Config
		wantCode   int
	}{
		{
			name: "lifo with valid configuration",
			args: []interface{}{
				10,
				15,
				"5s",
			},
			schedFunc: NewLIFO,
			wantName:  LIFOName,
			wantKey:   "mykey",
			wantErr:   false,
			wantConfig: scheduler.Config{
				MaxConcurrency: 10,
				MaxQueueSize:   15,
				Timeout:        5 * time.Second,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifo with partial invalid configuration, applies defaults",
			args: []interface{}{
				-1,
				-15,
			},
			schedFunc: NewLIFO,
			wantName:  LIFOName,
			wantKey:   "mykey",
			wantErr:   false,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   defaultMaxQueueSize,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifo with invalid configuration, does not create filter",
			args: []interface{}{
				-1,
				-15,
				"4a",
			},
			schedFunc: NewLIFO,
			wantName:  LIFOName,
			wantKey:   "mykey",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   defaultMaxQueueSize,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			l := tt.schedFunc()
			if l.Name() != tt.wantName {
				t.Errorf("Failed to get name, got %s, want %s", l.Name(), tt.wantName)
			}

			fl, err := l.CreateFilter(tt.args)
			if err != nil && !tt.wantErr {
				t.Fatalf("Failed to create filter: %v", err)
			}
			if tt.wantErr {
				t.Skip("want error on filter creation skip rest")
			}

			if f, ok := fl.(*lifoFilter); ok {
				if got := f.Config(); got != tt.wantConfig {
					t.Errorf("Failed to get Config, got: %v, want: %v", got, tt.wantConfig)
				}

			} else if !ok {
				t.Fatalf("Failed to get lifoFilter from filter: %v, ok: %v", f, ok)
			}

			backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {}))
			fr := make(filters.Registry)
			fr.Register(l)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: l.Name(), Args: tt.args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("lifo filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, tt.wantCode, r)
				buf := make([]byte, rsp.ContentLength)
				rsp.Body.Read(buf)
			}

		})
	}
}
