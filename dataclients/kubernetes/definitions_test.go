package kubernetes

import (
	"reflect"
	"testing"
)

func TestGetTargetPort(t *testing.T) {
	tests := []struct {
		name        string
		svc         *service
		svcPort     backendPort
		expected    string
		errExpected bool
	}{
		{
			name: "svc1",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						&servicePort{
							Port:       80,
							TargetPort: &backendPort{value: 5000},
						},
					},
				}},
			svcPort:     backendPort{value: 80},
			expected:    "80",
			errExpected: false,
		},
		{
			name: "svc without targetport",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						&servicePort{
							Port: 80,
						},
					},
				}},
			svcPort:     backendPort{value: 80},
			expected:    "",
			errExpected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.svc.GetTargetPort(tt.svcPort); got != tt.expected && (err != nil && !tt.errExpected) {
				t.Errorf("GetTargetPort: %v, expected: %v, err: %v", got, tt.expected, err)
			}
		})
	}
}

func TestMatchingPort(t *testing.T) {
	tests := []struct {
		name       string
		sp         *servicePort
		targetPort backendPort
		expected   bool
	}{
		{
			name: "svc-port",
			sp: &servicePort{
				Port:       80,
				TargetPort: &backendPort{value: 5000},
			},
			targetPort: backendPort{value: 80},
			expected:   true,
		},
		{
			name: "svc-name",
			sp: &servicePort{
				Name:       "web",
				Port:       80,
				TargetPort: &backendPort{value: 5000},
			},
			targetPort: backendPort{value: "web"},
			expected:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sp.MatchingPort(tt.targetPort); got != tt.expected {
				t.Errorf("MatchingPort: %v, expected: %v", got, tt.expected)
			}
		})
	}
}

func Test_endpoint_Targets(t *testing.T) {
	tests := []struct {
		name       string
		Subsets    []*subset
		ingSvcPort string
		targetPort *backendPort
		want       []string
	}{
		{
			name: "single node and port fully specified by name",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and port fully specified by port number",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by name",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						&port{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by port number",
			Subsets: []*subset{
				&subset{
					Addresses: []*address{
						&address{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						&port{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						&port{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &backendPort{value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := endpoint{
				Subsets: tt.Subsets,
			}
			if got := ep.Targets(tt.ingSvcPort, tt.targetPort.String()); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("endpoint.Targets() = %v, want %v", got, tt.want)
			}
		})
	}
}
