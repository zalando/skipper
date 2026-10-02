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
			newResourceID("default", "external-name-empty"): {
				Spec: &serviceSpec{Type: "ExternalName", ExternalName: ""},
			},
			newResourceID("default", "cluster-ip"): {
				Spec: &serviceSpec{Type: "ClusterIP", ClusterIP: "10.0.0.9"},
			},
			newResourceID("default", "cluster-ip-empty"): {
				Spec: &serviceSpec{Type: "ClusterIP", ClusterIP: ""},
			},
			newResourceID("default", "cluster-ip-none"): {
				Spec: &serviceSpec{Type: "ClusterIP", ClusterIP: "None"},
			},
			newResourceID("default", "node-port-external-ips"): {
				Spec: &serviceSpec{Type: "NodePort", ExternalIPs: []string{"198.51.100.1"}},
			},
			newResourceID("default", "node-port-endpoints"): {
				Spec: &serviceSpec{Type: "NodePort"},
			},
			newResourceID("default", "load-balancer"): {
				Spec: &serviceSpec{Type: "LoadBalancer"},
				Status: &serviceStatus{
					LoadBalancer: serviceLoadBalancerStatus{
						Ingress: []serviceLoadBalancerIngress{
							{IP: "1.2.3.4"},
							{Hostname: "lb.example.org"},
							{IP: "", Hostname: ""},
						},
					},
				},
			},
			newResourceID("default", "load-balancer-external-ips"): {
				Spec: &serviceSpec{Type: "LoadBalancer", ExternalIPs: []string{"198.51.100.2"}},
			},
			newResourceID("default", "load-balancer-empty"): {
				Spec: &serviceSpec{Type: "LoadBalancer"},
			},
			newResourceID("default", "custom-type-external-ips"): {
				Spec: &serviceSpec{Type: "CustomType", ExternalIPs: []string{"198.51.100.3"}},
			},
			newResourceID("default", "custom-type-empty"): {
				Spec: &serviceSpec{Type: "CustomType"},
			},
			newResourceID("default", "nil-spec"): {
				Spec: nil,
			},
		},
		endpoints: map[definitions.ResourceID]*endpoint{
			newResourceID("default", "node-port-endpoints"): {
				Subsets: []*subset{
					{
						Addresses: []*address{
							{IP: "10.2.0.1"},
						},
					},
				},
			},
		},
	}

	for _, tt := range []struct {
		name        string
		service     string
		zone        string
		expected    []definitions.IngressLoadBalancerIngress
		expectErr   bool
		errContains string
	}{
		{
			name:     "empty ingress status service config",
			service:  "",
			expected: nil,
		},
		{
			name:        "invalid namespace/name format",
			service:     "invalid-service-name",
			expectErr:   true,
			errContains: "invalid resource reference",
		},
		{
			name:        "service not found in cluster state",
			service:     "default/nonexistent-service",
			expectErr:   true,
			errContains: "service not found: default/nonexistent-service",
		},
		{
			name:        "service with nil spec returns error",
			service:     "default/nil-spec",
			expectErr:   true,
			errContains: "service not found: default/nil-spec",
		},
		{
			name:    "external name",
			service: "default/external-name",
			expected: []definitions.IngressLoadBalancerIngress{{
				Hostname: "example.org",
			}},
		},
		{
			name:     "external name empty returns nil",
			service:  "default/external-name-empty",
			expected: nil,
		},
		{
			name:    "cluster ip",
			service: "default/cluster-ip",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "10.0.0.9",
			}},
		},
		{
			name:     "cluster ip empty returns nil",
			service:  "default/cluster-ip-empty",
			expected: nil,
		},
		{
			name:     "cluster ip None (headless) returns nil",
			service:  "default/cluster-ip-none",
			expected: nil,
		},
		{
			name:    "node port with external IPs",
			service: "default/node-port-external-ips",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "198.51.100.1",
			}},
		},
		{
			name:    "node port fallback to endpoint addresses",
			service: "default/node-port-endpoints",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "10.2.0.1",
			}},
		},
		{
			name:    "load balancer with ingress status (filters empty entries)",
			service: "default/load-balancer",
			expected: []definitions.IngressLoadBalancerIngress{
				{IP: "1.2.3.4"},
				{Hostname: "lb.example.org"},
			},
		},
		{
			name:    "load balancer fallback to external IPs",
			service: "default/load-balancer-external-ips",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "198.51.100.2",
			}},
		},
		{
			name:     "load balancer without status or external IPs returns nil",
			service:  "default/load-balancer-empty",
			expected: nil,
		},
		{
			name:    "default/unknown service type with external IPs",
			service: "default/custom-type-external-ips",
			expected: []definitions.IngressLoadBalancerIngress{{
				IP: "198.51.100.3",
			}},
		},
		{
			name:     "default/unknown service type without external IPs returns nil",
			service:  "default/custom-type-empty",
			expected: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterClient{
				ingressStatusFromService: tt.service,
				zone:                     tt.zone,
			}
			addresses, err := c.ingressStatusAddressesFromService(state)
			if tt.expectErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, addresses)
		})
	}
}

func TestUpdateIngressesV1Status(t *testing.T) {
	t.Run("patches when status changed", func(t *testing.T) {
		var patched bool
		var patchedPath string
		var patchedPayload map[string]any

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

		statusObj, ok := patchedPayload["status"].(map[string]any)
		require.True(t, ok)
		loadBalancer, ok := statusObj["loadBalancer"].(map[string]any)
		require.True(t, ok)
		ingressEntries, ok := loadBalancer["ingress"].([]any)
		require.True(t, ok)
		require.Len(t, ingressEntries, 1)
	})
}
