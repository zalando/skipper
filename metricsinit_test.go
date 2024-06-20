package skipper

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"
)

// TODO: what is a more straightforward way to get an unused port?
func availablePort() (port int, err error) {
	var l net.Listener
	l, err = net.Listen("tcp", ":0")
	if err != nil {
		return
	}

	port = l.Addr().(*net.TCPAddr).Port
	l.Close()
	return
}

func mustAvailablePort(t *testing.T) int {
	p, err := availablePort()
	if err != nil {
		t.Error(t)
	}

	return p
}

// Initialization order of the metrics.Default global must be done before other packages may start to use it.
func TestInitOrderAndDefault(t *testing.T) {
	const (
		ringMetricsUpdatePeriod = time.Millisecond
		testTimeout             = 5 * time.Second
	)

	port := mustAvailablePort(t)
	supportPort := mustAvailablePort(t)
	redisPort := mustAvailablePort(t)
	sig := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		options := Options{
			Address:                       fmt.Sprintf(":%d", port),
			SupportListener:               fmt.Sprintf(":%d", supportPort),
			EnableRuntimeMetrics:          true,
			EnableSwarm:                   true,
			SwarmRedisURLs:                []string{fmt.Sprintf("localhost:%d", redisPort)},
			EnableRatelimiters:            true,
			SwarmRedisConnMetricsInterval: ringMetricsUpdatePeriod,
			PassiveHealthCheck: map[string]string{
				"period":               "1m",
				"min-requests":         "10",
				"max-drop-probability": "0.9",
				"min-drop-probability": "0.05",
			},
		}

		tornDown := make(chan struct{})
		if err := run(options, sig, tornDown); err != nil {
			t.Error(err)
		}

		<-tornDown
		close(done)
	}()

	to := time.After(testTimeout)
	func() {
		for {
			rsp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics/swarm", supportPort))
			if err != nil {
				t.Log("error making request", err)
			} else {
				rsp.Body.Close()
				if rsp.StatusCode == http.StatusOK {
					return
				}
			}

			select {
			case <-time.After(ringMetricsUpdatePeriod):
			case <-to:
				t.Error("test timeout")
				return
			}
		}
	}()

	sig <- syscall.SIGTERM
	<-done
}
