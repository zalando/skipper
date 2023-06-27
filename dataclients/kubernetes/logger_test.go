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
	manifest, err := os.Open("testdata/routegroups/convert/failing-filter.yaml")
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

	const testLoggingInterval = 1 * time.Second
	c.SetLoggingInterval(testLoggingInterval)

	_, err = c.LoadAll()
	require.NoError(t, err)

	assert.Equal(t, 1, countMessages(), "one message expected after initial load")

	for i := 0; i < 10; i++ {
		_, _, err := c.LoadUpdate()
		require.NoError(t, err)

		assert.Equal(t, 1, countMessages(), "one message expected on subsequent updates before logging interval elapsed")
	}

	time.Sleep(2 * testLoggingInterval)

	for i := 0; i < 10; i++ {
		_, _, err := c.LoadUpdate()
		require.NoError(t, err)

		assert.Equal(t, 2, countMessages(), "two messages expected on subsequent updates after logging interval elapsed")
	}

	oldLevel := log.GetLevel()
	defer log.SetLevel(oldLevel)

	log.SetLevel(log.DebugLevel)

	for i := 0; i < 10; i++ {
		_, _, err := c.LoadUpdate()
		require.NoError(t, err)

		assert.Equal(t, 3+i, countMessages(), "a new message expected for each subsequent update when log level is debug")
	}
}
