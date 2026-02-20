package kubernetes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestParseNamespaceName(t *testing.T) {
	for _, tt := range []struct {
		name      string
		resource  string
		expectErr bool
	}{
		{name: "valid", resource: "foo/bar"},
		{name: "missing namespace", resource: "bar", expectErr: true},
		{name: "missing name", resource: "foo/", expectErr: true},
		{name: "empty", resource: "", expectErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, err := parseNamespaceName(tt.resource)
			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, "foo", ns)
			require.Equal(t, "bar", name)
		})
	}
}

func TestIngressStatusAddressesFromService(t *testing.T) {
	state := &clusterState{
		services: map[definitions.ResourceID]*service{
			newResourceID("default", "external-name"): {
				Spec: &serviceSpec{Type: "ExternalName", ExternalName: "example.org"},
			},
			newResourceID("default", "cluster-ip"): {
				Spec: &serviceSpec{Type: "ClusterIP", ClusterIP: "10.0.0.9"},
			},
			newResourceID("default", "load-balancer"): {
				Spec:   &serviceSpec{Type: "LoadBalancer"},
				Status: &serviceStatus{LoadBalancer: serviceLoadBalancerStatus{Ingress: []serviceLoadBalancerIngress{{IP: "1.2.3.4"}}}},
			},
		},
	}

	for _, tt := range []struct {
		name     string
		service  string
		expected []definitions.IngressLoadBalancerIngress
	}{
		{
			name:    "external name",
			service: "default/external-name",
			expected: []definitions.IngressLoadBalancerIngress{{
				Hostname: "example.org",
			}},
		},
		{
			name:    "cluster ip",
			service: "default/cluster-ip",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "10.0.0.9",
			}},
		},
		{
			name:    "load balancer",
			service: "default/load-balancer",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "1.2.3.4",
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterClient{ingressStatusFromService: tt.service}
			addresses, err := c.ingressStatusAddressesFromService(state)
			require.NoError(t, err)
			require.Equal(t, tt.expected, addresses)
		})
	}
}

func TestUpdateIngressesV1Status(t *testing.T) {
	t.Run("patches when status changed", func(t *testing.T) {
		var patched bool
		var patchedPath string
		var patchedPayload map[string]interface{}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPatch, r.Method)
			patched = true
			patchedPath = r.URL.Path

			defer r.Body.Close()
			require.NoError(t, json.NewDecoder(r.Body).Decode(&patchedPayload))
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := &clusterClient{
			httpClient:               srv.Client(),
			apiURL:                   srv.URL,
			ingressStatusFromService: "default/publish-svc",
		}

		state := &clusterState{
			ingressesV1: []*definitions.IngressV1Item{{
				Metadata: &definitions.Metadata{Name: "test-ingress", Namespace: "default"},
				Status:   &definitions.IngressV1Status{},
			}},
			services: map[definitions.ResourceID]*service{
				newResourceID("default", "publish-svc"): {
					Spec:   &serviceSpec{Type: "LoadBalancer"},
					Status: &serviceStatus{LoadBalancer: serviceLoadBalancerStatus{Ingress: []serviceLoadBalancerIngress{{IP: "1.2.3.4"}}}},
				},
			},
		}

		err := c.updateIngressesV1Status(state)
		require.NoError(t, err)
		require.True(t, patched)
		require.Equal(t, "/apis/networking.k8s.io/v1/namespaces/default/ingresses/test-ingress/status", patchedPath)

		statusObj, ok := patchedPayload["status"].(map[string]interface{})
		require.True(t, ok)
		loadBalancer, ok := statusObj["loadBalancer"].(map[string]interface{})
		require.True(t, ok)
		ingressEntries, ok := loadBalancer["ingress"].([]interface{})
		require.True(t, ok)
		require.Len(t, ingressEntries, 1)
	})

	t.Run("skips patch when status unchanged", func(t *testing.T) {
		var patchCount int

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			patchCount++
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := &clusterClient{
			httpClient:               srv.Client(),
			apiURL:                   srv.URL,
			ingressStatusFromService: "default/publish-svc",
		}

		state := &clusterState{
			ingressesV1: []*definitions.IngressV1Item{{
				Metadata: &definitions.Metadata{Name: "test-ingress", Namespace: "default"},
				Status: &definitions.IngressV1Status{LoadBalancer: definitions.IngressV1LoadBalancerStatus{Ingress: []definitions.IngressLoadBalancerIngress{{
					IP: "1.2.3.4",
				}}}},
			}},
			services: map[definitions.ResourceID]*service{
				newResourceID("default", "publish-svc"): {
					Spec:   &serviceSpec{Type: "LoadBalancer"},
					Status: &serviceStatus{LoadBalancer: serviceLoadBalancerStatus{Ingress: []serviceLoadBalancerIngress{{IP: "1.2.3.4"}}}},
				},
			},
		}

		err := c.updateIngressesV1Status(state)
		require.NoError(t, err)
		require.Equal(t, 0, patchCount)
	})

	t.Run("updates regardless of ingress class object", func(t *testing.T) {
		var patchCount int

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPatch:
				patchCount++
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c := &clusterClient{
			httpClient:               srv.Client(),
			apiURL:                   srv.URL,
			ingressStatusFromService: "default/publish-svc",
		}

		state := &clusterState{
			ingressesV1: []*definitions.IngressV1Item{{
				Metadata: &definitions.Metadata{Name: "test-ingress", Namespace: "default"},
				Spec:     &definitions.IngressV1Spec{IngressClassName: "some-class"},
				Status:   &definitions.IngressV1Status{},
			}},
			services: map[definitions.ResourceID]*service{
				newResourceID("default", "publish-svc"): {
					Spec:   &serviceSpec{Type: "LoadBalancer"},
					Status: &serviceStatus{LoadBalancer: serviceLoadBalancerStatus{Ingress: []serviceLoadBalancerIngress{{IP: "1.2.3.4"}}}},
				},
			},
		}

		err := c.updateIngressesV1Status(state)
		require.NoError(t, err)
		require.Equal(t, 1, patchCount)
	})
}
