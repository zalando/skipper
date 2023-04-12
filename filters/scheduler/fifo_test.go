package scheduler

import (
	"fmt"
	"io"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestCreateFifoFilter(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []interface{}
		wantParseErr bool
	}{
		{
			name:         "fifo no args",
			wantParseErr: true,
		},
		{
			name: "fifo 1 arg",
			args: []interface{}{
				3,
			},
			wantParseErr: true,
		},
		{
			name: "fifo 2 args",
			args: []interface{}{
				3,
				5,
			},
			wantParseErr: true,
		},
		{
			name: "fifo simple ok 3 args",
			args: []interface{}{
				3,
				5,
				"1s",
			},
		},
		{
			name: "fifo wrong type arg1",
			args: []interface{}{
				"3",
				5,
				"1s",
			},
			wantParseErr: true,
		},
		{
			name: "fifo wrong type arg2",
			args: []interface{}{
				3,
				"5",
				"1s",
			},
			wantParseErr: true,
		},
		{
			name: "fifo wrong time.Duration string arg3",
			args: []interface{}{
				3,
				5,
				"1sa",
			},
			wantParseErr: true,
		},
		{
			name: "fifo wrong type arg3",
			args: []interface{}{
				3,
				5,
				1,
			},
			wantParseErr: true,
		},
		{
			name: "fifo too many args",
			args: []interface{}{
				3,
				5,
				"1s",
				"foo",
			},
			wantParseErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := &fifoSpec{}
			ff, err := spec.CreateFilter(tt.args)
			if err != nil && !tt.wantParseErr {
				t.Fatalf("Failed to parse filter: %v", err)
			}
			if err == nil && tt.wantParseErr {
				t.Fatal("Failed to get wanted error on create filter")
			}

			if _, ok := ff.(*fifoFilter); !ok && err == nil {
				t.Fatal("Failed to convert filter to *fifoFilter")
			}
		})
	}
}

func TestFifo(t *testing.T) {
	for _, tt := range []struct {
		name          string
		args          []interface{}
		freq          int
		per           time.Duration
		backendTime   time.Duration
		clientTimeout time.Duration
		wantConfig    scheduler.Config
		wantParseErr  bool
		wantOkRate    float64
		epsilon       float64
	}{
		{
			name:         "fifo defaults",
			args:         []interface{}{},
			wantParseErr: true,
		},
		{
			name: "fifo simple ok",
			args: []interface{}{
				3,
				5,
				"1s",
			},
			freq:          20,
			per:           100 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantConfig: scheduler.Config{
				MaxConcurrency: 3,
				MaxQueueSize:   5,
				Timeout:        time.Second,
			},
			wantParseErr: false,
			wantOkRate:   1.0,
			epsilon:      1,
		},
		{
			name: "fifo with reaching max concurrency and queue timeouts",
			args: []interface{}{
				3,
				5,
				"10ms",
			},
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   10 * time.Millisecond,
			clientTimeout: time.Second,
			wantConfig: scheduler.Config{
				MaxConcurrency: 3,
				MaxQueueSize:   5,
				Timeout:        10 * time.Millisecond,
			},
			wantParseErr: false,
			wantOkRate:   0.1,
			epsilon:      1,
		},
		{
			name: "fifo with reaching max concurrency and queue full",
			args: []interface{}{
				1,
				1,
				"250ms",
			},
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   100 * time.Millisecond,
			clientTimeout: time.Second,
			wantConfig: scheduler.Config{
				MaxConcurrency: 1,
				MaxQueueSize:   1,
				Timeout:        250 * time.Millisecond,
			},
			wantParseErr: false,
			wantOkRate:   0.001,
			epsilon:      1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewFifo()
			if fs.Name() != filters.FifoName {
				t.Fatalf("Failed to get name got %s want %s", fs.Name(), filters.FifoName)
			}

			// no parse error
			ff, err := fs.CreateFilter(tt.args)
			if err != nil && !tt.wantParseErr {
				t.Fatalf("Failed to parse filter: %v", err)
			}
			if err == nil && tt.wantParseErr {
				t.Fatalf("want parse error but hav no: %v", err)
			}
			if tt.wantParseErr {
				return
			}

			// validate config
			if f, ok := ff.(*fifoFilter); ok {
				config := f.Config()
				if config != tt.wantConfig {
					t.Fatalf("Failed to get Config, got: %v, want: %v", config, tt.wantConfig)
				}
				if f.queue != f.GetQueue() {
					t.Fatal("Failed to get expected queue")
				}
			}

			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteFIFOMetrics: true,
			})
			defer reg.Close()

			fr := make(filters.Registry)
			fr.Register(fs)

			backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.backendTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			var fmtStr string
			switch len(tt.args) {
			case 0:
				fmtStr = `aroute: * -> fifo() -> "%s"`
			case 1:
				fmtStr = `aroute: * -> fifo(%v) -> "%s"`
			case 2:
				fmtStr = `aroute: * -> fifo(%v, %v) -> "%s"`
			case 3:
				fmtStr = `aroute: * -> fifo(%v, %v, "%v") -> "%s"`
			default:
				t.Fatalf("Test not possible %d >3", len(tt.args))
			}

			args := append(tt.args, backend.URL)
			doc := fmt.Sprintf(fmtStr, args...)
			t.Logf("%s", doc)

			dc, err := testdataclient.NewDoc(doc)
			if err != nil {
				t.Fatalf("Failed to create testdataclient: %v", err)
			}
			defer dc.Close()

			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{dc},
				PostProcessors:  []routing.PostProcessor{reg},
			}
			rt := routing.New(ro)
			defer rt.Close()
			<-rt.FirstLoad()

			tracer := &testTracer{MockTracer: mocktracer.New()}
			pr := proxy.WithParams(proxy.Params{
				Routing:     rt,
				OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
			})
			defer pr.Close()

			ts := stdlibhttptest.NewServer(pr)
			defer ts.Close()

			reqURL, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", ts.URL, err)
			}

			rsp, err := http.DefaultClient.Get(reqURL.String())
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", reqURL.String(), err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}

			va := httptest.NewVegetaAttacker(reqURL.String(), tt.freq, tt.per, tt.clientTimeout)
			va.Attack(io.Discard, 1*time.Second, tt.name)

			t.Logf("Success [0..1]: %0.2f", va.Success())
			t.Logf("requests: %d", va.TotalRequests())
			got := va.TotalSuccess()
			want := tt.wantOkRate * float64(va.TotalRequests())
			if got < want {
				t.Fatalf("OK rate too low got<want: %0.0f < %0.0f", got, want)
			}
			countOK, ok := va.CountStatus(http.StatusOK)
			if !ok && tt.wantOkRate > 0 {
				t.Fatal("no OK")
			}
			if !ok && tt.wantOkRate == 0 {
				count499, ok := va.CountStatus(0)
				if !ok || va.TotalRequests() != uint64(count499) {
					t.Fatalf("want all 499 client cancel but %d != %d", va.TotalRequests(), count499)
				}
			}
			if float64(countOK) < want {
				t.Fatalf("OK too low got<want: %d < %0.0f", countOK, want)
			}
		})
	}
}

func TestConstantRouteUpdatesFifo(t *testing.T) {
	for _, tt := range []struct {
		name          string
		args          []interface{}
		freq          int
		per           time.Duration
		updateRate    time.Duration
		backendTime   time.Duration
		clientTimeout time.Duration
		wantConfig    scheduler.Config
		wantParseErr  bool
		wantOkRate    float64
		epsilon       float64
	}{
		{
			name: "fifo simple ok",
			args: []interface{}{
				3,
				5,
				"1s",
			},
			freq:          20,
			per:           100 * time.Millisecond,
			updateRate:    25 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantConfig: scheduler.Config{
				MaxConcurrency: 3,
				MaxQueueSize:   5,
				Timeout:        time.Second,
			},
			wantParseErr: false,
			wantOkRate:   1.0,
			epsilon:      1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewFifo()
			if fs.Name() != filters.FifoName {
				t.Fatalf("Failed to get name got %s want %s", fs.Name(), filters.FifoName)
			}

			// no parse error
			ff, err := fs.CreateFilter(tt.args)
			if err != nil && !tt.wantParseErr {
				t.Fatalf("Failed to parse filter: %v", err)
			}
			if err == nil && tt.wantParseErr {
				t.Fatalf("want parse error but hav no: %v", err)
			}
			if tt.wantParseErr {
				return
			}

			// validate config
			if f, ok := ff.(*fifoFilter); ok {
				config := f.Config()
				if config != tt.wantConfig {
					t.Fatalf("Failed to get Config, got: %v, want: %v", config, tt.wantConfig)
				}
				if f.queue != f.GetQueue() {
					t.Fatal("Failed to get expected queue")
				}
			}

			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteFIFOMetrics: true,
			})
			defer reg.Close()

			fr := make(filters.Registry)
			fr.Register(fs)

			backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.backendTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			args := append(tt.args, backend.URL)
			doc := fmt.Sprintf(`aroute: * -> fifo(%v, %v, "%v") -> "%s"`, args...)

			dc, err := testdataclient.NewDoc(doc)
			if err != nil {
				t.Fatalf("Failed to create testdataclient: %v", err)
			}
			defer dc.Close()

			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{dc},
				PostProcessors:  []routing.PostProcessor{reg},
			}
			rt := routing.New(ro)
			defer rt.Close()
			<-rt.FirstLoad()

			tracer := &testTracer{MockTracer: mocktracer.New()}
			pr := proxy.WithParams(proxy.Params{
				Routing:     rt,
				OpenTracing: &proxy.OpenTracingParams{Tracer: tracer},
			})
			defer pr.Close()

			ts := stdlibhttptest.NewServer(pr)
			defer ts.Close()

			reqURL, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", ts.URL, err)
			}

			rsp, err := http.DefaultClient.Get(reqURL.String())
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", reqURL.String(), err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}

			// run dataclient updates
			quit := make(chan struct{})
			newDoc := fmt.Sprintf(`aroute: * -> fifo(100, 200, "250ms") -> "%s"`, backend.URL)
			go func(q chan<- struct{}, updateRate time.Duration, doc1, doc2 string) {
				i := 0
				for {
					select {
					case <-quit:
						println("number of route updates:", i)
						return
					case <-time.After(updateRate):
					}
					i++
					if i%2 == 0 {
						dc.UpdateDoc(doc2, nil)
					} else {
						dc.UpdateDoc(doc1, nil)
					}
				}

			}(quit, tt.updateRate, doc, newDoc)

			va := httptest.NewVegetaAttacker(reqURL.String(), tt.freq, tt.per, tt.clientTimeout)
			va.Attack(io.Discard, 1*time.Second, tt.name)
			quit <- struct{}{}

			t.Logf("Success [0..1]: %0.2f", va.Success())
			t.Logf("requests: %d", va.TotalRequests())
			got := va.TotalSuccess()
			want := tt.wantOkRate * float64(va.TotalRequests())
			if got < want {
				t.Fatalf("OK rate too low got<want: %0.0f < %0.0f", got, want)
			}
			countOK, ok := va.CountStatus(http.StatusOK)
			if !ok && tt.wantOkRate > 0 {
				t.Fatalf("no OK")
			}
			if !ok && tt.wantOkRate == 0 {
				count499, ok := va.CountStatus(0)
				if !ok || va.TotalRequests() != uint64(count499) {
					t.Fatalf("want all 499 client cancel but %d != %d", va.TotalRequests(), count499)
				}
			}
			if float64(countOK) < want {
				t.Fatalf("OK too low got<want: %d < %0.0f", countOK, want)
			}
		})
	}
}

func TestFifoClientTimeout(t *testing.T) {
	metrics := &metricstest.MockMetrics{}
	reg := scheduler.RegistryWith(scheduler.Options{
		Metrics:                metrics,
		EnableRouteFIFOMetrics: true,
	})
	defer reg.Close()

	fr := make(filters.Registry)
	fr.Register(NewFifo())
	fr.Register(flowid.New())

	t.Logf("set backend timeout to 200ms")
	backendTime := 200 * time.Millisecond
	backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/change-timeout" {
			s := r.URL.Query().Get("d")
			d, err := time.ParseDuration(s)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("ERR"))
				return
			}
			backendTime = d

		} else {
			time.Sleep(backendTime)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	args := []interface{}{10, 300, "3s"}
	doc := fmt.Sprintf(`
r: * -> fifo(%v, %v, "%v") -> flowId() -> <loopback>;
s: HeaderRegexp("X-Flow-Id", ".*") -> fifo(%v, %v, "%v") -> "%s";
be: Path("/change-timeout") -> "%s";`, append(append(args, args...), backend.URL, backend.URL)...)

	dc, err := testdataclient.NewDoc(doc)
	if err != nil {
		t.Fatalf("Failed to create testdataclient: %v", err)
	}
	defer dc.Close()

	ro := routing.Options{
		SignalFirstLoad: true,
		FilterRegistry:  fr,
		DataClients:     []routing.DataClient{dc},
		PostProcessors:  []routing.PostProcessor{reg},
	}
	rt := routing.New(ro)
	defer rt.Close()
	<-rt.FirstLoad()

	tracer := &testTracer{MockTracer: mocktracer.New()}
	pr := proxy.WithParams(proxy.Params{
		Routing:           rt,
		OpenTracing:       &proxy.OpenTracingParams{Tracer: tracer},
		AccessLogDisabled: true,
	})
	defer pr.Close()

	ts := stdlibhttptest.NewServer(pr)
	defer ts.Close()

	reqURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("Failed to parse url %s: %v", ts.URL, err)
	}

	rsp, err := http.DefaultClient.Get(reqURL.String())
	if err != nil {
		t.Fatalf("Failed to get response from %s: %v", reqURL.String(), err)
	}
	rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
	}

	// step1: slow backend leads to client timeouts
	t.Log("step1: backend is slow, so we assume we get only 499 client timeout")
	va := httptest.NewVegetaAttacker(reqURL.String(), 200, 100*time.Millisecond, 50*time.Millisecond)
	va.Attack(io.Discard, 2*time.Second, "client timeout")
	t.Logf("Success [0..1]: %0.2f", va.Success())
	t.Logf("requests: %d", va.TotalRequests())
	count500, ok := va.CountStatus(500)
	t.Logf("500s: %d", count500)
	count502, ok := va.CountStatus(502)
	t.Logf("502s: %d", count502)
	count503, ok := va.CountStatus(503)
	t.Logf("503s: %d", count503)
	count499, ok := va.CountStatus(0)
	t.Logf("499s: %d", count499)
	if !ok || va.TotalRequests() != uint64(count499) {
		t.Fatalf("want a lot 499 client cancel but %d != %d", va.TotalRequests(), uint64(count499))
	}

	// step2: heal backend
	t.Log("step2: set backend timeout to 2ms")
	rsp, err = http.DefaultClient.Get(reqURL.String() + "/change-timeout?d=2ms")
	if err != nil {
		t.Fatalf("Failed to get response from %s: %v", reqURL.String()+"/change-timeout?d=2ms", err)
	}
	defer rsp.Body.Close()

	// step3: healthy backend leads to 200 OK
	t.Log("step3: after backend is fine again we assume we get only 200 OK")
	va = httptest.NewVegetaAttacker(reqURL.String(), 200, 100*time.Millisecond, 50*time.Millisecond)
	va.Attack(io.Discard, 2*time.Second, "client should not timeout")
	total := va.TotalRequests()
	t.Logf("Success [0..1]: %0.2f", va.Success())
	t.Logf("requests: %d", total)
	countOK, ok := va.CountStatus(http.StatusOK)
	if !ok || uint64(countOK) != total {
		t.Fatalf("want to OK: %d != %d", countOK, total)
	}
	count499, _ = va.CountStatus(0)
	if count499 > 0 {
		t.Fatalf("want no 499 client cancel but %d > 0", count499)
	}
}
