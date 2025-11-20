package scheduler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aryszka/jobqueue"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
	"github.com/zalando/skipper/tracing/tracingtest"
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
			wantName:  filters.LifoName,
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
			wantName:  filters.LifoGroupName,
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
			wantName:  filters.LifoGroupName,
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
			name: "lifo with invalid first arg fails",
			args: []interface{}{
				"a",
				0,
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantErr:   true,
		},
		{
			name: "lifo with invalid second arg fails",
			args: []interface{}{
				0,
				"a",
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantErr:   true,
		},
		{
			name: "lifo with invalid third arg fails",
			args: []interface{}{
				0,
				0,
				"a",
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantErr:   true,
		},
		{
			name: "lifogroup with invalid first arg fails",
			args: []interface{}{
				5,
				0,
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantErr:   true,
		},
		{
			name: "lifogroup with invalid third arg fails",
			args: []interface{}{
				"foo",
				0,
				"a",
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantErr:   true,
		},
		{
			name: "lifo with too many args fails",
			args: []interface{}{
				0,
				0,
				"1s",
				"foo",
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantErr:   true,
		},
		{
			name: "lifoGroup with too many args fails",
			args: []interface{}{
				"foo",
				0,
				0,
				"1s",
				"foo",
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantErr:   true,
		},
		{
			name:      "lifoGroup with no args fails",
			args:      []interface{}{},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantErr:   true,
		},
		{
			name: "lifo with partial invalid configuration, applies defaults",
			args: []interface{}{
				0,
				0,
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantKey:   "mykey",
			wantErr:   false,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   0,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with partial invalid configuration, applies defaults",
			args: []interface{}{
				"mygroup",
				0,
				0,
				"-1s",
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
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
				0,
				0,
				"4a",
			},
			schedFunc: NewLIFO,
			wantName:  filters.LifoName,
			wantKey:   "mykey",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   0,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with invalid configuration, does not create filter",
			args: []interface{}{
				"mygroup",
				0,
				0,
				"4a",
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantKey:   "mygroup",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   0,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with invalid duration type, does not create filter",
			args: []interface{}{
				"mygroup",
				0,
				0,
				4.5,
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantKey:   "mygroup",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   0,
				Timeout:        defaultTimeout,
			},
			wantCode: http.StatusOK,
		},
		{
			name: "lifogroup with invalid int type, does not create filter",
			args: []interface{}{
				"mygroup",
				"foo",
				0,
				"4s",
			},
			schedFunc: NewLIFOGroup,
			wantName:  filters.LifoGroupName,
			wantKey:   "mygroup",
			wantErr:   true,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   0,
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
			if err == nil && tt.wantErr {
				t.Fatal("Failed to get wanted error on create filter")
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
				t.Fatalf("Failed to get lifoFilter or lifoGroupFilter from filter: %v, ok: %v", f, ok)
			}

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.wantCode)
				w.Write([]byte("Hello"))
			}))
			defer backend.Close()

			args := append(tt.args, backend.URL)
			args = append([]interface{}{l.Name()}, args...)

			var doc string
			switch len(args) {
			case 2:
				doc = fmt.Sprintf(`aroute: * -> %s() -> "%s"`, args...)
			case 3:
				doc = fmt.Sprintf(`aroute: * -> %s(%v) -> "%s"`, args...)
			case 4:
				doc = fmt.Sprintf(`aroute: * -> %s(%v, %v) -> "%s"`, args...)
			case 5:
				doc = fmt.Sprintf(`aroute: * -> %s(%v, %v, "%v") -> "%s"`, args...)
			case 6:
				doc = fmt.Sprintf(`aroute: * -> %s("%v", %v, %v, "%v") -> "%s"`, args...)
			default:
				t.Fatalf("(%d): %v", len(args), args)
			}
			println("doc:", doc)
			t.Logf("doc: %s", doc)

			dc, err := testdataclient.NewDoc(doc)
			if err != nil {
				t.Fatalf("Failed to create testdataclient: %v", err)
			}
			defer dc.Close()

			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteLIFOMetrics: true,
			})
			defer reg.Close()

			fr := make(filters.Registry)
			fr.Register(l)

			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{dc},
				PostProcessors:  []routing.PostProcessor{reg},
			}
			rt := routing.New(ro)
			defer rt.Close()
			<-rt.FirstLoad()

			pr := proxy.WithParams(proxy.Params{Routing: rt})
			defer pr.Close()

			ts := httptest.NewServer(pr)
			defer ts.Close()

			reqURL, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", ts.URL, err)
			}

			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < config.MaxQueueSize+config.MaxConcurrency+1; i++ {
				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
				}

				defer rsp.Body.Close()

				if rsp.StatusCode != tt.wantCode {
					t.Errorf("lifo filter failed got=%d, expected=%d", rsp.StatusCode, tt.wantCode)
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

func TestLifoErrors(t *testing.T) {

	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
	}))
	defer backend.Close()

	doc := fmt.Sprintf(`aroute: * -> lifo(5, 7, "100ms") -> "%s"`, backend.URL)

	dc, err := testdataclient.NewDoc(doc)
	require.NoError(t, err)
	defer dc.Close()

	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteLIFOMetrics: true,
	})
	defer reg.Close()

	fr := make(filters.Registry)
	fr.Register(NewLIFO())

	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PostProcessors:  []routing.PostProcessor{reg},
	}

	rt := routing.New(ro)
	defer rt.Close()

	<-rt.FirstLoad()

	tracer := tracingtest.NewTracer()
	pr := proxy.WithParams(proxy.Params{
		Routing:     rt,
		OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
	})
	defer pr.Close()

	ts := httptest.NewServer(pr)
	defer ts.Close()

	requestSpike(t, 20, ts.URL)

	codes := make(map[uint16]int)
	for _, span := range tracer.FinishedSpans() {
		if span.OperationName == "ingress" {
			code := span.Tag("http.status_code").(uint16)
			codes[code]++
			if code >= 500 {
				assert.Equal(t, true, span.Tag("error"))
			} else {
				assert.Nil(t, span.Tag("error"))
			}
		}
	}

	assert.Equal(t, map[uint16]int{
		// 20 request in total, of which:
		200: 5, // went straight to the backend
		502: 7, // were queued and timed out as backend latency is greater than scheduling timeout
		503: 8, // were refused due to full queue
	}, codes)

	reg.UpdateMetrics()

	metrics.WithCounters(func(counters map[string]int64) {
		assert.Equal(t, int64(7), counters["lifo.aroute.error.timeout"])
		assert.Equal(t, int64(8), counters["lifo.aroute.error.full"])
	})
}

func requestSpike(t *testing.T, n int, url string) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			rsp, err := http.Get(url)
			require.NoError(t, err)
			defer rsp.Body.Close()
			wg.Done()
		}()
	}
	wg.Wait()
}
