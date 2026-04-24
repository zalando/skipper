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

func containsCount(s, substr string, count int) bool {
	var found int
	for {
		i := strings.Index(s, substr)
		if i < 0 {
			return found == count
		}

		if found == count {
			return false
		}

		found++
		s = s[i+len(substr):]
	}
}

func containsEveryLineCount(s, substr string, count int) bool {
	l := strings.SplitSeq(substr, "\n")
	for li := range l {
		if !containsCount(s, li, count) {
			return false
		}
	}

	return true
}

func TestMissingRouteGroupsCRDLoggedOnlyOnce(t *testing.T) {
	a, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{FindNot: []string{kubernetes.ZalandoResourcesClusterURI}})
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(a)
	defer s.Close()

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	c, err := kubernetes.New(kubernetes.Options{KubernetesURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.LoadAll(); err != nil {
		t.Fatal(err)
	}

	if _, err := c.LoadAll(); err != nil {
		t.Fatal(err)
	}

	if !containsEveryLineCount(logBuf.String(), kubernetes.RouteGroupsNotInstalledMessage, 1) {
		t.Error("missing RouteGroups CRD was not reported exactly once")
	}
}

func TestLoadRouteGroups(t *testing.T) {

	for _, tt := range []struct {
		msg     string
		rgClass string
		spec    string
		loads   bool
	}{{
		msg:     "annotation set, and matches class",
		rgClass: "test",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations:
    zalando.org/routegroup.class: test
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: true,
	}, {
		msg:     "annotation set, and class doesn't match",
		rgClass: "test",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations:
    zalando.org/routegroup.class: incorrectclass
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: false,
	}, {
		msg:     "no annotation is loaded",
		rgClass: "test",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations: {}
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: true,
	}, {
		msg:     "empty annotation is loaded",
		rgClass: "test",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations:
    zalando.org/routegroup.class: ""
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: true,
	}, {
		msg:     "annotation matches regexp class, route group loads",
		rgClass: "^test.*",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations:
    zalando.org/routegroup.class: testing
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: true,
	}, {
		msg:     "annotation doesn't matches regexp class, route group isn't loaded",
		rgClass: "^test.*",
		spec: `
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  annotations:
    zalando.org/routegroup.class: a-test
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
`,
		loads: false,
	}} {

		t.Run(tt.msg, func(t *testing.T) {
			a, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, bytes.NewBufferString(tt.spec))
			if err != nil {
				t.Error(err)
			}

			s := httptest.NewServer(a)
			defer s.Close()

			c, err := kubernetes.New(kubernetes.Options{KubernetesURL: s.URL, RouteGroupClass: tt.rgClass})
			if err != nil {
				t.Error(err)
			}
			defer c.Close()

			rgs, err := c.ClusterClient.LoadRouteGroups()
			if err != nil {
				t.Error(err)
			}

			if tt.loads != (len(rgs) == 1) {
				t.Errorf("mismatch when loading route groups. Expected loads: %t, actual %t", tt.loads, (len(rgs) == 1))
			}
		})
	}
}

func TestLoggingInterval(t *testing.T) {
	// TODO: with validation changes we need to update/refactor this test
	manifest, err := os.Open("testdata/routegroups/convert/missing-service.yaml")
	require.NoError(t, err)
	defer manifest.Close()

	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)

	countMessages := func() int {
		return strings.Count(out.String(), "Ignoring route: default/myapp: service not found")
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
