package kubernetes_test

import (
	"bytes"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func TestLogger(t *testing.T) {
	// TODO: with validattion changes we need to update/refactor this test
	manifest, err := os.Open("testdata/routegroups/convert/missing-service.yaml")
	require.NoError(t, err)
	defer manifest.Close()

	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)

	countMessages := func() int {
		return strings.Count(out.String(), "Error transforming external hosts")
	}

	a, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, manifest)
	require.NoError(t, err)

	s := httptest.NewServer(a)
	defer s.Close()

	c, err := kubernetes.New(kubernetes.Options{KubernetesURL: s.URL})
	require.NoError(t, err)
	defer c.Close()

	const loggingInterval = 100 * time.Millisecond
	c.SetLoggingInterval(loggingInterval)

	_, err = c.LoadAll()
	require.NoError(t, err)

	assert.Equal(t, 1, countMessages(), "one message expected after initial load")

	const (
		n              = 2
		updateDuration = time.Duration(n)*loggingInterval + loggingInterval/2
	)

	start := time.Now()
	for time.Since(start) < updateDuration {
		_, _, err := c.LoadUpdate()
		require.NoError(t, err)

		time.Sleep(loggingInterval / 10)
	}

	assert.Equal(t, 1+n, countMessages(), "%d additional messages expected", n)

	oldLevel := log.GetLevel()
	defer log.SetLevel(oldLevel)

	log.SetLevel(log.DebugLevel)

	for i := 1; i <= 10; i++ {
		_, _, err := c.LoadUpdate()
		require.NoError(t, err)

		assert.Equal(t, 1+n+i, countMessages(), "a new message expected for each subsequent update when log level is debug")
	}
}
