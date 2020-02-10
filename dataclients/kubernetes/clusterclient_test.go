package kubernetes

import (
	"bytes"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

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

	_, err = c.LoadAll()
	_, err = c.LoadAll()

	logString := logBuf.String()
	if strings.Index(logString, routeGroupsNotInstalledMessage) < 0 {
		t.Error("failed to log missing RouteGroups CRD")
	}

	if strings.Index(logString, routeGroupsNotInstalledMessage) !=
		strings.LastIndex(logString, routeGroupsNotInstalledMessage) {
		t.Error("missing RouteGroups CRD was reported multiple times")
	}
}
