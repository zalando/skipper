package kubernetes

import (
	"bytes"
	"net/http/httptest"
	"os"
	"regexp"
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

func TestSkipRouteGroups(t *testing.T) {

	ingClsRx, err := regexp.Compile("")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Name            string
		RouteGroupClass string
		Annotations     map[string]string
		Skipped         bool
	}{
		{
			Name:            "class-matches",
			RouteGroupClass: "test",
			Annotations: map[string]string{
				"zalando.org/routegroup.class": "test",
			},
			Skipped: false,
		},
		{
			Name:            "class-doesnt-match",
			RouteGroupClass: "test",
			Annotations: map[string]string{
				"zalando.org/routegroup.class": "nottheclass",
			},
			Skipped: true,
		},
		{
			Name:            "no-class-matches",
			RouteGroupClass: "test",
			Annotations:     map[string]string{},
			Skipped:         false,
		},
		{
			Name:            "class-regexp-matches",
			RouteGroupClass: "^test.*",
			Annotations: map[string]string{
				"zalando.org/routegroup.class": "testing",
			},
			Skipped: false,
		},
		{
			Name:            "class-regexp-doesnt-match",
			RouteGroupClass: "^test.*",
			Annotations: map[string]string{
				"zalando.org/routegroup.class": "a-test",
			},
			Skipped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {

			rgClsRx, err := regexp.Compile(tt.RouteGroupClass)
			if err != nil {
				t.Fatal(err)
			}

			c := &clusterClient{
				ingressesURI:    ingressesClusterURI,
				routeGroupsURI:  routeGroupsClusterURI,
				servicesURI:     servicesClusterURI,
				endpointsURI:    endpointsClusterURI,
				ingressClass:    ingClsRx,
				routeGroupClass: rgClsRx,
			}

			item := &routeGroupItem{
				Metadata: &metadata{
					Name:        "rg",
					Annotations: tt.Annotations,
				},
				Spec: &routeGroupSpec{
					DefaultBackends: []*backendReference{
						{
							BackendName: "test",
						},
					},
					Backends: []*skipperBackend{
						{
							Name:    "test",
							Address: "localhost",
						},
					},
				},
			}

			if c.skipRouteGroup(item) != tt.Skipped {
				t.Error("routegroup filtered incorrectly")
			}
		})
	}
}
