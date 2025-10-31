package tee_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/tee"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics/metricstest"
	teePredicate "github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type testLog struct {
	mu sync.Mutex

	buf      bytes.Buffer
	oldOut   io.Writer
	oldLevel log.Level
}

func NewTestLog() *testLog {
	oldOut := log.StandardLogger().Out
	oldLevel := log.GetLevel()
	log.SetLevel(log.DebugLevel)

	tl := &testLog{oldOut: oldOut, oldLevel: oldLevel}
	log.SetOutput(tl)
	return tl
}

func (l *testLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(p)
}

func (l *testLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.String()
}

func (l *testLog) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf.Reset()
}

func (l *testLog) Close() {
	log.SetOutput(l.oldOut)
	log.SetLevel(l.oldLevel)
}

func (l *testLog) WaitForN(exp string, n int, to time.Duration) error {
	timeout := time.After(to)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for log entry: %s", exp)
		case <-ticker.C:
			if l.Count(exp) >= n {
				return nil
			}
		}
	}
}

func (l *testLog) WaitFor(exp string, to time.Duration) error {
	return l.WaitForN(exp, 1, to)
}

func (l *testLog) Count(exp string) int {
	return strings.Count(l.String(), exp)
}

func TestTeeRedirectLoop(t *testing.T) {
	for _, tt := range []struct {
		filterName string
	}{
		{
			filterName: "tee",
		},
		{
			filterName: "teenf",
		}} {
		t.Run(tt.filterName, func(t *testing.T) {

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(200)
				w.Write([]byte("backend"))
			}))
			defer backend.Close()

			target := ""
			shadow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req, err := http.NewRequest("GET", target, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				if val := r.Header.Get(tee.ShadowTrafficHeader); val != "" {
					req.Header.Set(tee.ShadowTrafficHeader, val)
				}

				rsp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("Failed to GET %q: %v", target, err)
				}
				t.Logf("response: %d", rsp.StatusCode)

				io.Copy(io.Discard, r.Body)
				io.Copy(io.Discard, rsp.Body)
				w.WriteHeader(200)
				w.Write([]byte("shadow"))
			}))
			defer shadow.Close()
			defer http.DefaultClient.CloseIdleConnections()

			doc := fmt.Sprintf(`route1: * -> %s("%s") -> "%s";`, tt.filterName, backend.URL, shadow.URL)

			fr := builtin.MakeRegistry()
			dc, err := testdataclient.NewDoc(doc)
			if err != nil {
				t.Fatalf("Failed to create dataclient: %v", err)
			}
			defer dc.Close()

			mockMetrics := &metricstest.MockMetrics{}
			defer mockMetrics.Close()
			tl := loggingtest.New()
			defer tl.Close()

			proxyLog := NewTestLog()
			defer proxyLog.Close()

			ro := routing.Options{
				FilterRegistry: fr,
				DataClients:    []routing.DataClient{dc},
				Log:            tl,
				Metrics:        mockMetrics,
				PollTimeout:    100 * time.Millisecond,
				Predicates:     []routing.PredicateSpec{teePredicate.New()},
			}

			rt := routing.New(ro)
			defer rt.Close()

			pr := proxy.WithParams(proxy.Params{
				CloseIdleConnsPeriod: time.Second,
				Metrics:              mockMetrics,
				Routing:              rt,
			})
			defer pr.Close()
			p := httptest.NewServer(pr)
			defer p.Close()

			// create the loop: shadow to proxy
			target = p.URL

			req, err := http.NewRequest("GET", p.URL, nil)
			if err != nil {
				t.Error(err)
			}

			rsp, err := p.Client().Do(req)
			if err != nil {
				t.Error(err)
			}
			mockMetrics.WithCounters(func(counters map[string]int64) {
				for k, v := range counters {
					t.Logf("%s: %d", k, v)
					if k == "incoming.HTTP/1.1" && v > 2 {
						t.Fatalf("Failed to mitigate request incoming: %d", v)
					}
					if k == "outgoing.HTTP/1.1" && v > 1 {
						t.Fatalf("Failed to mitigate duplicate request outgoing: %d", v)
					}
				}

			})

			rsp.Body.Close()

			req, err = http.NewRequest("GET", p.URL, nil)
			if err != nil {
				t.Error(err)
			}
			req.Header.Set("X-Skipper-Shadow-Traffic", "test")
			rsp, err = p.Client().Do(req)
			if err != nil {
				t.Error(err)
			}
			defer rsp.Body.Close()
			if rsp.StatusCode != 465 {
				t.Fatalf("Failed to detect loop: %d", rsp.StatusCode)

			}
		})
	}
}
