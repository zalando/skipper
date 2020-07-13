package kubernetes

import (
	"reflect"
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestGetTargetPort(t *testing.T) {
	tests := []struct {
		name        string
		svc         *service
		svcPort     definitions.BackendPort
		expected    string
		errExpected bool
	}{
		{
			name: "svc1",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						{
							Port:       80,
							TargetPort: &definitions.BackendPort{Value: 5000},
						},
					},
				}},
			svcPort:     definitions.BackendPort{Value: 80},
			expected:    "80",
			errExpected: false,
		},
		{
			name: "svc without targetport",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						{
							Port: 80,
						},
					},
				}},
			svcPort:     definitions.BackendPort{Value: 80},
			expected:    "",
			errExpected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.svc.getTargetPort(tt.svcPort); got != tt.expected && (err != nil && !tt.errExpected) {
				t.Errorf("getTargetPort: %v, expected: %v, err: %v", got, tt.expected, err)
			}
		})
	}
}

func TestMatchingPort(t *testing.T) {
	tests := []struct {
		name       string
		sp         *servicePort
		targetPort definitions.BackendPort
		expected   bool
	}{
		{
			name: "svc-port",
			sp: &servicePort{
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 5000},
			},
			targetPort: definitions.BackendPort{Value: 80},
			expected:   true,
		},
		{
			name: "svc-name",
			sp: &servicePort{
				Name:       "web",
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 5000},
			},
			targetPort: definitions.BackendPort{Value: "web"},
			expected:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sp.matchingPort(tt.targetPort); got != tt.expected {
				t.Errorf("matchingPort: %v, expected: %v", got, tt.expected)
			}
		})
	}
}

func Test_endpoint_Targets(t *testing.T) {
	tests := []struct {
		name       string
		Subsets    []*subset
		ingSvcPort string
		targetPort *definitions.BackendPort
		want       []string
	}{
		{
			name: "single node and port fully specified by name",
			Subsets: []*subset{
				{
					Addresses: []*address{
						{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &definitions.BackendPort{Value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and port fully specified by port number",
			Subsets: []*subset{
				{
					Addresses: []*address{
						{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &definitions.BackendPort{Value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by name",
			Subsets: []*subset{
				{
					Addresses: []*address{
						{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "http",
			targetPort: &definitions.BackendPort{Value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
		{
			name: "single node and 2 ports fully specified by port number",
			Subsets: []*subset{
				{
					Addresses: []*address{
						{
							IP:   "1.2.3.4",
							Node: "nodeA",
						},
					},
					Ports: []*port{
						{
							Name:     "http",
							Port:     80,
							Protocol: "tcp",
						},
						{
							Name:     "metrics",
							Port:     9911,
							Protocol: "tcp",
						},
					},
				},
			},
			ingSvcPort: "80",
			targetPort: &definitions.BackendPort{Value: 80},
			want:       []string{"http://1.2.3.4:80"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := endpoint{
				Subsets: tt.Subsets,
			}
			if got := ep.targets(tt.ingSvcPort, tt.targetPort.String(), "http"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("endpoint.targets() = %v, want %v", got, tt.want)
			}
		})
	}
}
