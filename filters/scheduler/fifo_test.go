package scheduler

import (
	"fmt"
	"io"
	"net/http"
	stdlibhttptest "net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/httptest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestCreateFifoName(t *testing.T) {
	for _, tt := range []struct {
		name       string
		filterFunc func() filters.Spec
	}{
		{
			name:       filters.FifoName,
			filterFunc: NewFifo,
		},
		{
			name:       filters.FifoWithBodyName,
			filterFunc: NewFifoWithBody,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.filterFunc().Name() != tt.name {
				t.Fatalf("got %q, want %q", tt.filterFunc().Name(), tt.name)
			}
		})
	}
}

func TestCreateFifoFilter(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []interface{}
		wantParseErr bool
		wantConfig   scheduler.Config
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
			wantConfig: scheduler.Config{
				MaxConcurrency: 3,
				MaxQueueSize:   5,
				Timeout:        1 * time.Second,
			},
		},
		{
			name: "fifo negative value arg1",
			args: []interface{}{
				-3,
				5,
				"1s",
			},
			wantParseErr: true,
		},
		{
			name: "fifo negative value arg2",
			args: []interface{}{
				3,
				-5,
				"1s",
			},
			wantParseErr: true,
		},
		{
			name: "fifo too small value arg3",
			args: []interface{}{
				3,
				5,
				"1ns",
			},
			wantParseErr: true,
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
			for _, f := range []func() filters.Spec{NewFifo, NewFifoWithBody} {
				spec := f()
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
			}
		})
	}
}

func TestFifoWithBody(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []interface{}
		backendTime  time.Duration
		responseSize int
	}{
		{
			name:         "fifoWithBody 1024",
			args:         []interface{}{1, 0, "1s"},
			backendTime:  10 * time.Millisecond,
			responseSize: 1024,
		},
		{
			name:         "fifoWithBody 100MB",
			args:         []interface{}{1, 0, "20ms"},
			backendTime:  10 * time.Millisecond,
			responseSize: 100 * 1000 * 1024,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {

			backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				// sleep here to test the difference between streaming response and not
				time.Sleep(tt.backendTime)
				// TODO: maybe better to do slow body streaming?
				w.Write([]byte(strings.Repeat("A", tt.responseSize)))
			}))
			defer backend.Close()

			// proxy
			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteFIFOMetrics: true,
			})
			defer reg.Close()
			fr := make(filters.Registry)
			fr.Register(NewFifoWithBody())
			args := append(tt.args, backend.URL)
			doc := fmt.Sprintf(`r: * -> fifoWithBody(%v, %v, "%v") -> "%s"`, args...)
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

			// simple test
			rsp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", ts.URL, err)
			}
			defer rsp.Body.Close()
			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}
			b, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body from: %v", err)
			}
			if len(b) != tt.responseSize {
				t.Fatalf("Failed to read the size, got: %v, want: %v", len(b), tt.responseSize)
			}

			// the streaming test
			rspCH := make(chan *http.Response)
			errCH := make(chan error)
			waithCH := make(chan struct{})
			go func() {
				rsp, err := ts.Client().Get(ts.URL)
				waithCH <- struct{}{}
				if err != nil {
					errCH <- err
				} else {
					rspCH <- rsp
				}
			}()

			<-waithCH
			rsp, err = ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("Failed to do 2nd request: %v", err)
			} else {
				b, err := io.ReadAll(rsp.Body)
				if err != nil {
					t.Fatalf("Failed 2nd request to read body: %v", err)
				}
				if len(b) != tt.responseSize {
					t.Fatalf("Failed 2nd request to get response size: %d, want: %d", len(b), tt.responseSize)
				}
			}
			select {
			case err := <-errCH:
				t.Fatalf("Failed to do request: %v", err)
			case rsp := <-rspCH:
				b, err := io.ReadAll(rsp.Body)
				if err != nil {
					t.Fatalf("Failed to read body: %v", err)
				}
				if len(b) != tt.responseSize {
					t.Fatalf("Failed to get response size: %d, want: %d", len(b), tt.responseSize)
				}
			}

		})
	}
}

func TestFifo(t *testing.T) {
	for _, tt := range []struct {
		name          string
		filter        string
		freq          int
		per           time.Duration
		backendTime   time.Duration
		clientTimeout time.Duration
		wantOkRate    float64
	}{
		{
			name:        "fifo simple ok",
			filter:      `fifo(3, 5, "1s")`,
			freq:        20,
			per:         100 * time.Millisecond,
			backendTime: 1 * time.Millisecond,
			wantOkRate:  1.0,
		},
		{
			name:          "fifoWithbody simple ok",
			filter:        `fifoWithbody(3, 5, "1s")`,
			freq:          20,
			per:           100 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    1.0,
		},
		{
			name:          "fifo with reaching max concurrency and queue timeouts",
			filter:        `fifo(3, 5, "10ms")`,
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   10 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    0.1,
		},
		{
			name:          "fifoWithbody with reaching max concurrency and queue timeouts",
			filter:        `fifoWithbody(3, 5, "10ms")`,
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   10 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    0.1,
		},
		{
			name:          "fifo with reaching max concurrency and queue full",
			filter:        `fifo(1, 1, "250ms")`,
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   100 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    0.0008,
		},
		{
			name:          "fifoWithBody with reaching max concurrency and queue full",
			filter:        `fifoWithBody(1, 1, "250ms")`,
			freq:          200,
			per:           100 * time.Millisecond,
			backendTime:   100 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    0.0008,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteFIFOMetrics: true,
			})
			defer reg.Close()

			fr := make(filters.Registry)
			fr.Register(NewFifo())
			fr.Register(NewFifoWithBody())

			backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.backendTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			if ff := eskip.MustParseFilters(tt.filter); len(ff) != 1 {
				t.Fatalf("expected one filter, got %d", len(ff))
			}

			doc := fmt.Sprintf(`aroute: * -> %s -> "%s"`, tt.filter, backend.URL)
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

			rsp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", ts.URL, err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}

			va := httptest.NewVegetaAttacker(ts.URL, tt.freq, tt.per, tt.clientTimeout)
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

func TestFifoConstantRouteUpdates(t *testing.T) {
	for _, tt := range []struct {
		name          string
		filter        string
		freq          int
		per           time.Duration
		updateRate    time.Duration
		backendTime   time.Duration
		clientTimeout time.Duration
		wantOkRate    float64
	}{
		{
			name:          "fifo simple ok",
			filter:        `fifo(3, 5, "1s")`,
			freq:          20,
			per:           100 * time.Millisecond,
			updateRate:    25 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    1.0,
		}, {
			name:          "fifoWithBody simple ok",
			filter:        `fifoWithBody(3, 5, "1s")`,
			freq:          20,
			per:           100 * time.Millisecond,
			updateRate:    25 * time.Millisecond,
			backendTime:   1 * time.Millisecond,
			clientTimeout: time.Second,
			wantOkRate:    1.0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &metricstest.MockMetrics{}
			reg := scheduler.RegistryWith(scheduler.Options{
				Metrics:                metrics,
				EnableRouteFIFOMetrics: true,
			})
			defer reg.Close()

			fr := make(filters.Registry)
			fr.Register(NewFifo())
			fr.Register(NewFifoWithBody())

			backend := stdlibhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.backendTime)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer backend.Close()

			if ff := eskip.MustParseFilters(tt.filter); len(ff) != 1 {
				t.Fatalf("expected one filter, got %d", len(ff))
			}

			doc := fmt.Sprintf(`aroute: * -> %s -> "%s"`, tt.filter, backend.URL)
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

			rsp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("Failed to get response from %s: %v", ts.URL, err)
			}
			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Fatalf("Failed to get valid response from endpoint: %d", rsp.StatusCode)
			}

			// run dataclient updates
			quit := make(chan struct{})
			newDoc := fmt.Sprintf(`aroute: * -> %s -> "%s"`, tt.filter, backend.URL)
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

			va := httptest.NewVegetaAttacker(ts.URL, tt.freq, tt.per, tt.clientTimeout)
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
