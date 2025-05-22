package swarm

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func newFakeKubernetesNodeInfoClient(url string) nodeInfoClient {
	cli, err := kubernetes.New(kubernetes.Options{
		KubernetesURL: url,
	})
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %v", err)
	}

	_, err = cli.LoadAll()
	if err != nil {
		log.Fatalf("Failed LoadAll kubernetes client: %v", err)
	}

	return &nodeInfoClientKubernetes{
		client:    cli,
		name:      defaultName,
		namespace: DefaultNamespace,
		port:      DefaultPort,
	}
}

func TestGetKubeNodeInfo(t *testing.T) {
	kubernetestest.FixturesToTest(
		t,
		"testdata/endpointslice",
	)

	// s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	// 	w.Write([]byte(content))
	// }))
	// defer s.Close()
	// c := newFakeKubernetesNodeInfoClient(s.URL)
	// infos, err := c.GetNodeInfo()
	// if err != nil {
	// 	t.Fatalf("Failed to get nodeinfos: %v", err)
	// }
	// if len(infos) < 1 {
	// 	t.Errorf("Failed to get nodeinfos: %d", len(infos))
	// }
}
