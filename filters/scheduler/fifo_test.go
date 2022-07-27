package scheduler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"

	vegeta "github.com/tsenart/vegeta/lib"
)

func TestCreateFifoFilter(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []interface{}
		wantParseErr bool
	}{
		{
			name: "fifo simple ok no args",
		},
		{
			name: "fifo simple ok 1 arg",
			args: []interface{}{
				3,
			},
		},
		{
			name: "fifo simple ok 2 args",
			args: []interface{}{
				3,
				5,
			},
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
			name:          "fifo defaults",
			args:          []interface{}{},
			freq:          20,
			per:           100 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantConfig: scheduler.Config{
				MaxConcurrency: defaultMaxConcurreny,
				MaxQueueSize:   defaultMaxQueueSize,
				Timeout:        defaultTimeout,
			},
			wantParseErr: false,
			wantOkRate:   1.0,
			epsilon:      1,
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

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.backendTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))

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

			ts := httptest.NewServer(pr)
			defer ts.Close()

			reqURL, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", ts.URL, err)
			}

			rsp, err := http.DefaultClient.Get(reqURL.String())
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", reqURL.String(), err)
			}
			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}

			va := newVegetaAttacker(reqURL.String(), tt.freq, tt.per, tt.clientTimeout)
			// buf := bytes.NewBuffer(make([]byte, 0, 1024))
			// va.Attack(buf, 3*time.Second, tt.name)
			va.Attack(io.Discard, 1*time.Second, tt.name)
			//t.Logf("buf: %v", buf.String())

			t.Logf("Success [0..1]: %0.2f", va.metrics.Success)
			t.Logf("requests: %d", va.metrics.Requests)
			got := va.metrics.Success * float64(va.metrics.Requests)
			want := tt.wantOkRate * float64(va.metrics.Requests)
			if got < want {
				t.Fatalf("OK rate too low got<want: %0.0f < %0.0f", got, want)
			}
			countOK, ok := va.metrics.StatusCodes["200"]
			if !ok && tt.wantOkRate > 0 {
				t.Fatal("no OK")
			}
			if !ok && tt.wantOkRate == 0 {
				count499, ok := va.metrics.StatusCodes["0"]
				if !ok || va.metrics.Requests != uint64(count499) {
					t.Fatalf("want all 499 client cancel but %d != %d", va.metrics.Requests, count499)
				}
			}
			if float64(countOK) < want {
				t.Fatalf("OK too low got<want: %d < %0.0f", countOK, want)
			}
		})
	}
}

type vegetaAttacker struct {
	attacker *vegeta.Attacker
	metrics  *vegeta.Metrics
	rate     *vegeta.Rate
	targeter vegeta.Targeter
}

func newVegetaAttacker(url string, freq int, per time.Duration, timeout time.Duration) *vegetaAttacker {
	atk := vegeta.NewAttacker(
		vegeta.Connections(10),
		vegeta.H2C(false),
		vegeta.HTTP2(false),
		vegeta.KeepAlive(true),
		vegeta.MaxWorkers(10),
		vegeta.Redirects(0),
		vegeta.Timeout(timeout),
		vegeta.Workers(5),
	)

	tr := vegeta.NewStaticTargeter(vegeta.Target{Method: "GET", URL: url})
	rate := vegeta.Rate{Freq: freq, Per: per}

	m := vegeta.Metrics{
		Histogram: &vegeta.Histogram{
			Buckets: []time.Duration{
				0,
				10 * time.Microsecond,
				50 * time.Microsecond,
				100 * time.Microsecond,
				500 * time.Microsecond,
				1 * time.Millisecond,
				5 * time.Millisecond,
				10 * time.Millisecond,
				25 * time.Millisecond,
				50 * time.Millisecond,
				100 * time.Millisecond,
				1000 * time.Millisecond,
			},
		},
	}

	return &vegetaAttacker{
		attacker: atk,
		metrics:  &m,
		rate:     &rate,
		targeter: tr,
	}
}

func (atk *vegetaAttacker) Attack(w io.Writer, d time.Duration, name string) {
	for res := range atk.attacker.Attack(atk.targeter, atk.rate, d, name) {
		if res == nil {
			continue
		}
		atk.metrics.Add(res)
		//metrics.Latencies.Add(res.Latency)
	}
	atk.metrics.Close()
	// logrus.Info("histogram reporter:")
	// histReporter := vegeta.NewHistogramReporter(atk.metrics.Histogram)
	// histReporter.Report(os.Stdout)
	log.Print("text reporter:")
	reporter := vegeta.NewTextReporter(atk.metrics)
	reporter.Report(w)
}
