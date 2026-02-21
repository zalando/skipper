package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestUpdateOnlyChangedRoutes(t *testing.T) {
	api := newTestAPIWithEndpoints(t, &serviceList{}, &definitions.IngressV1List{}, &endpointList{}, &secretList{})
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

	assert.EqualValues(t, healthcheckRoutes(false), r, "no healthcheck routes received")

	for range 3 {
		update, del, err := k.LoadUpdate()
		if err != nil {
			t.Fatal(err)
		}

		if len(update) != 0 || len(del) != 0 {
			t.Fatal("unexpected update received")
		}
	}
}
