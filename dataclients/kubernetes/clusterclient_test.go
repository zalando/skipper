package kubernetes

import (
	"bytes"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func containsMax(s, substr string, maxCount int) bool {
	var count int
	for {
		i := strings.Index(s, substr)
		if i < 0 {
			return count <= maxCount
		}

		if maxCount < 0 {
			return true
		}

		if count == maxCount {
			return false
		}

		count++
		s = s[i+len(substr):]
	}
}

func containsEveryLineMax(s, substr string, maxCount int) bool {
	l := strings.Split(substr, "\n")
	for _, li := range l {
		if !containsMax(s, li, maxCount) {
			return false
		}
	}

	return true
}

func TestMissingRouteGroupsCRDLoggedOnlyOnce(t *testing.T) {
	a, err := newAPI(testAPIOptions{FindNot: []string{clusterZalandoResourcesURI}})
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(a)
	defer s.Close()

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	c, err := New(Options{KubernetesURL: s.URL})
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

	if !containsEveryLineMax(logBuf.String(), routeGroupsNotInstalledMessage, 1) {
		t.Error("missing RouteGroups CRD was not reported exactly once")
	}
}
