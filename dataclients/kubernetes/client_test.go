package kubernetes_test

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

func newTestClient(t *testing.T, opts kubernetes.Options, specPath string) *kubernetes.Client {
	t.Helper()

	yaml, err := os.Open(specPath)
	require.NoError(t, err)
	defer yaml.Close()

	api, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, yaml)
	require.NoError(t, err)

	apiServer := httptest.NewServer(api)
	t.Cleanup(apiServer.Close)

	opts.KubernetesURL = apiServer.URL

	client, err := kubernetes.New(opts)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}

func TestClientGetEndpointAddresses(t *testing.T) {
	t.Run("from empty state", func(t *testing.T) {
		client := newTestClient(t,
			kubernetes.Options{},
			"testdata/ingressV1/ingress-data/lb-target-multi.yaml",
		)

		// client.LoadAll() is not called

		addrs := client.GetEndpointAddresses("", "namespace1", "service1")
		assert.Nil(t, addrs)
	})

	t.Run("from endpoints", func(t *testing.T) {
		client := newTestClient(t,
			kubernetes.Options{},
			"testdata/ingressV1/ingress-data/lb-target-multi.yaml",
		)

		_, err := client.LoadAll()
		require.NoError(t, err)

		addrs := client.GetEndpointAddresses("", "namespace1", "service1")
		assert.Equal(t, []string{"42.0.1.2", "42.0.1.3"}, addrs)

		// test subsequent call returns the expected values even when previous result was modified
		addrs[0] = "modified"

		addrs = client.GetEndpointAddresses("", "namespace1", "service1")
		assert.Equal(t, []string{"42.0.1.2", "42.0.1.3"}, addrs)
	})

	t.Run("from endpointslices", func(t *testing.T) {
		client := newTestClient(t,
			kubernetes.Options{
				KubernetesEnableEndpointslices: true,
			},
			"testdata/ingressV1/ingress-data/lb-target-multi-multiple-endpointslices-conditions-all-ready.yaml",
		)

		_, err := client.LoadAll()
		require.NoError(t, err)

		addrs := client.GetEndpointAddresses("", "namespace1", "service1")
		assert.Equal(t, []string{"42.0.1.1", "42.0.1.2", "42.0.1.3", "42.0.1.4"}, addrs)

		// test subsequent call returns the expected values even when previous result was modified
		addrs[0] = "modified"

		addrs = client.GetEndpointAddresses("", "namespace1", "service1")
		assert.Equal(t, []string{"42.0.1.1", "42.0.1.2", "42.0.1.3", "42.0.1.4"}, addrs)
	})
}

func TestClientLoadEndpointAddresses(t *testing.T) {
	t.Run("from endpoints", func(t *testing.T) {
		client := newTestClient(t,
			kubernetes.Options{},
			"testdata/ingressV1/ingress-data/lb-target-multi.yaml",
		)

		addrs, err := client.LoadEndpointAddresses("", "namespace1", "service1")
		assert.NoError(t, err)
		assert.Equal(t, []string{"42.0.1.2", "42.0.1.3"}, addrs)
	})

	t.Run("from endpointslices", func(t *testing.T) {
		client := newTestClient(t,
			kubernetes.Options{
				KubernetesEnableEndpointslices: true,
			},
			"testdata/ingressV1/ingress-data/lb-target-multi-multiple-endpointslices-conditions-all-ready.yaml",
		)

		addrs, err := client.LoadEndpointAddresses("", "namespace1", "service1")
		assert.NoError(t, err)
		assert.Equal(t, []string{"42.0.1.1", "42.0.1.2", "42.0.1.3", "42.0.1.4"}, addrs)
	})
}
