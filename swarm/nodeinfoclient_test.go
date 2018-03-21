package swarm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
)

func newFakeNodeInfoClient(url string) *NodeInfoClient {
	cli, err := NewClient(false, url)
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %v", err)
	}

	return &NodeInfoClient{
		kubernetesInCluster: false,
		kubeAPIBaseURL:      url,
		client:              cli,
		namespace:           DefaultNamespace,
		labelKey:            "application",
		labelVal:            "skipper-ingress",
		port:                DefaultSwarmPort,
	}
}

func TestGetKubeNodeInfo(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(content))
	}))
	defer s.Close()
	c := newFakeNodeInfoClient(s.URL)
	infos, err := c.GetNodeInfo()
	if err != nil {
		t.Errorf("Failed to get nodeinfos: %v", err)
		return
	}
	if len(infos) < 1 {
		t.Errorf("Failed to get nodeinfos: %d", len(infos))
	}
}
