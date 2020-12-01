package kubernetes

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestUpdateOnlyChangedRoutes(t *testing.T) {
	api := newTestAPIWithEndpoints(t, &serviceList{}, &definitions.IngressList{}, &endpointList{})
	defer api.Close()

	k, err := New(Options{
		KubernetesURL:      api.server.URL,
		ProvideHealthcheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer k.Close()

	r, err := k.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(r) != 1 || r[0].Id != healthcheckRouteID {
		t.Fatal("no healthcheck route received")
	}

	for i := 0; i < 3; i++ {
		update, del, err := k.LoadUpdate()
		if err != nil {
			t.Fatal(err)
		}

		if len(update) != 0 || len(del) != 0 {
			t.Fatal("unexpected udpate received")
		}
	}
}
