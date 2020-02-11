package kubernetes

import (
	"bytes"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
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
	l := strings.Split(substr, "\n")
	for _, li := range l {
		if !containsCount(s, li, count) {
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

	if !containsEveryLineCount(logBuf.String(), routeGroupsNotInstalledMessage, 1) {
		t.Error("missing RouteGroups CRD was not reported exactly once")
	}
}
