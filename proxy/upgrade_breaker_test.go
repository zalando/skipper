package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/metrics/metricstest"
	"golang.org/x/net/websocket"
)

type upgradeBackendMetrics struct {
	metricstest.MockMetrics
	backendErrors atomic.Int64
}

func (m *upgradeBackendMetrics) IncErrorsBackend(string) {
	m.backendErrors.Add(1)
}

func TestUpgradeCircuitBreaker(t *testing.T) {
	for _, ti := range []struct {
		name          string
		statuses      []int
		failures      int
		open          bool
		truncated     bool
		backendErrors int64
	}{
		{name: "successful upgrade", statuses: []int{101}, failures: 1},
		{name: "rejected upgrade", statuses: []int{400}, failures: 1},
		{name: "backend error", statuses: []int{503}, failures: 1, open: true, backendErrors: 1},
		{name: "connection error", statuses: []int{0}, failures: 1, open: true, backendErrors: 1},
		{name: "incomplete response", statuses: []int{200}, failures: 1, open: true, truncated: true, backendErrors: 1},
		{name: "success resets failures", statuses: []int{503, 101, 503}, failures: 2, backendErrors: 2},
	} {
		t.Run(ti.name, func(t *testing.T) {
			var status atomic.Int64
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/ws" {
					io.WriteString(w, "healthy")
					return
				}
				if code := int(status.Load()); code != http.StatusSwitchingProtocols {
					if ti.truncated {
						w.Header().Set("Content-Length", strconv.Itoa(len("backend response")+1))
					}
					w.WriteHeader(code)
					io.WriteString(w, "backend response")
					return
				}
				websocket.Handler(func(ws *websocket.Conn) {
					ws.SetDeadline(time.Now().Add(time.Second))
					var message string
					if err := websocket.Message.Receive(ws, &message); err != nil {
						t.Error(err)
						return
					}
					if err := websocket.Message.Send(ws, message); err != nil {
						t.Error(err)
					}
				}).ServeHTTP(w, r)
			}))
			defer backend.Close()
			if ti.statuses[0] == 0 {
				backend.Close()
			}

			m := &upgradeBackendMetrics{}
			tp, err := newTestProxyWithParams(fmt.Sprintf(`route: * -> "%s";`, backend.URL), Params{
				ExperimentalUpgrade: true,
				Metrics:             m,
				CircuitBreakers: circuit.NewRegistry(circuit.BreakerSettings{
					Type: circuit.ConsecutiveFailures, Failures: ti.failures, Timeout: time.Minute,
				}),
			})
			require.NoError(t, err)
			defer tp.close()
			completed := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tp.proxy.ServeHTTP(w, r)
				if r.URL.Path == "/ws" {
					completed <- struct{}{}
				}
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = time.Second

			for _, code := range ti.statuses {
				status.Store(int64(code))
				if code == http.StatusSwitchingProtocols {
					ws, err := websocket.Dial(strings.Replace(server.URL, "http:", "ws:", 1)+"/ws", "", server.URL)
					require.NoError(t, err)
					defer ws.Close()
					require.NoError(t, ws.SetDeadline(time.Now().Add(time.Second)))
					require.NoError(t, websocket.Message.Send(ws, "hello"))
					var message string
					require.NoError(t, websocket.Message.Receive(ws, &message))
					require.Equal(t, "hello", message)
					require.Equal(t, io.EOF, websocket.Message.Receive(ws, &message))
					ws.Close()
				} else {
					req, err := http.NewRequest(http.MethodGet, server.URL+"/ws", nil)
					require.NoError(t, err)
					req.Header.Set("Connection", "Upgrade")
					req.Header.Set("Upgrade", "websocket")
					resp, err := client.Do(req)
					require.NoError(t, err)
					resp.Body.Close()
					if code == 0 {
						code = http.StatusServiceUnavailable
					}
					require.Equal(t, code, resp.StatusCode)
				}
				select {
				case <-completed:
				case <-time.After(time.Second):
					t.Fatal("upgrade handler did not complete")
				}
			}

			if got := m.backendErrors.Load(); got != ti.backendErrors {
				t.Errorf("backend error count: got %d, want %d", got, ti.backendErrors)
			}
			resp, err := client.Get(server.URL + "/health")
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			if ti.open {
				if resp.StatusCode != http.StatusServiceUnavailable {
					t.Errorf("follow-up request status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
				}
				require.Equal(t, "true", resp.Header.Get("X-Circuit-Open"))
			} else {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("follow-up request status: got %d, want %d", resp.StatusCode, http.StatusOK)
				} else {
					require.Equal(t, "healthy", string(body))
				}
			}
		})
	}
}
