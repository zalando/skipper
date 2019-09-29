package scheduler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aryszka/jobqueue"
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
			name: "lifogroup with valid configuration",
			args: []interface{}{
				"mygroup",
				10,
				15,
				"5s",
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
			wantErr:   false,
			wantConfig: scheduler.Config{
				MaxConcurrency: 10,
				MaxQueueSize:   15,
				Timeout:        5 * time.Second,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with valid float64 configuration",
			args: []interface{}{
				"mygroup",
				10.1,
				15.2,
				"5s",
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
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
			name: "lifogroup with partial invalid configuration, applies defaults",
			args: []interface{}{
				"mygroup",
				-1,
				-15,
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
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
		},
		{
			name: "lifogroup with invalid configuration, does not create filter",
			args: []interface{}{
				"mygroup",
				-1,
				-15,
				"4a",
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   defaultMaxQueueSize,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with invalid duration type, does not create filter",
			args: []interface{}{
				"mygroup",
				-1,
				-15,
				4.5,
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   defaultMaxQueueSize,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with invalid int type, does not create filter",
			args: []interface{}{
				"mygroup",
				"foo",
				-15,
				"4s",
			},
			schedFunc: NewLIFOGroup,
			wantName:  LIFOGroupName,
			wantKey:   "mygroup",
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

			var (
				config scheduler.Config
				queue  *scheduler.Queue
			)

			if f, ok := fl.(*lifoFilter); ok {
				config = f.Config()
				if config != tt.wantConfig {
					t.Errorf("Failed to get Config, got: %v, want: %v", config, tt.wantConfig)
				}
				queue = f.queue

			} else if fg, ok := fl.(*lifoGroupFilter); ok {
				got := fg.Config()
				if got != tt.wantConfig {
					t.Errorf("Failed to get Config, got: %v, want: %v", got, tt.wantConfig)
				}
				config = got
				queue = fg.queue

				if !fg.HasConfig() {
					t.Errorf("Failed to HasConfig, got: %v", got)
				}
				if fg.Group() != tt.wantKey {
					t.Errorf("Failed to get Group, got: %v, want: %v", fg.Group(), tt.wantKey)
				}
				if q := fg.GetQueue(); q != nil {
					t.Errorf("Queue should be nil, got: %v", q)
				}
				fg.SetQueue(&scheduler.Queue{})
				if q := fg.GetQueue(); q == nil {
					t.Errorf("Queue should not be nil, got: %v", q)
				}
			} else {
				t.Fatalf("Failed to get lifoFilter ot lifoGroupFilter from filter: %v, ok: %v", f, ok)
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

			for i := 0; i < config.MaxQueueSize+config.MaxConcurrency+1; i++ {
				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
				}

				defer rsp.Body.Close()

				if rsp.StatusCode != tt.wantCode {
					t.Errorf("lifo filter failed got=%d, expected=%d, route=%s", rsp.StatusCode, tt.wantCode, r)
					buf := make([]byte, rsp.ContentLength)
					if n, err := rsp.Body.Read(buf); err != nil || int64(n) != rsp.ContentLength {
						t.Errorf("Failed to read content: %v, %d, want: %d", err, int64(n), rsp.ContentLength)
					}
				}
			}

			if queue != nil {
				// should be blocked
				rsp, err := http.DefaultClient.Do(req)
				if err != jobqueue.ErrStackFull {
					t.Errorf("Failed to get expected error: %v", err)
				}
				if rsp.StatusCode != http.StatusServiceUnavailable {
					t.Errorf("Wrong http status code got %d, expected: %d", rsp.StatusCode, http.StatusServiceUnavailable)
				}

				defer rsp.Body.Close()
			}
		})
	}
}
