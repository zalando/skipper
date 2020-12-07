package kubernetes

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestGetServicePort(t *testing.T) {
	tests := []struct {
		name        string
		svc         *service
		svcPort     definitions.BackendPort
		expected    *servicePort
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
				},
			},
			svcPort: definitions.BackendPort{Value: 80},
			expected: &servicePort{
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 5000},
			},
			errExpected: false,
		},
		{
			name: "named service port",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						{
							Name:       "web",
							Port:       80,
							TargetPort: &definitions.BackendPort{Value: 5000},
						},
					},
				},
			},
			svcPort: definitions.BackendPort{Value: "web"},
			expected: &servicePort{
				Name:       "web",
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 5000},
			},
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
				},
			},
			svcPort:     definitions.BackendPort{Value: 80},
			expected:    nil,
			errExpected: true,
		},
		{
			name: "multiple service ports, by value",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						{
							Name:       "web",
							Port:       80,
							TargetPort: &definitions.BackendPort{Value: 8080},
						},
						{
							Name:       "metrics",
							Port:       81,
							TargetPort: &definitions.BackendPort{Value: 8181},
						},
					},
				},
			},
			svcPort: definitions.BackendPort{Value: 80},
			expected: &servicePort{
				Name:       "web",
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 8080},
			},
			errExpected: false,
		},
		{
			name: "multiple service ports, by name",
			svc: &service{
				Spec: &serviceSpec{
					Ports: []*servicePort{
						{
							Name:       "web",
							Port:       80,
							TargetPort: &definitions.BackendPort{Value: 8080},
						},
						{
							Name:       "metrics",
							Port:       81,
							TargetPort: &definitions.BackendPort{Value: 8181},
						},
					},
				},
			},
			svcPort: definitions.BackendPort{Value: "web"},
			expected: &servicePort{
				Name:       "web",
				Port:       80,
				TargetPort: &definitions.BackendPort{Value: 8080},
			},
			errExpected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.svc.getServicePort(tt.svcPort)
			if err != nil && !tt.errExpected {
				t.Errorf("did not expect err, but got: %v", err)
			}
			if err == nil && tt.errExpected {
				t.Errorf("expected err, but got: %v, getServicePort: %v", err, got)
			}
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("err: %v\n%s", err, diff)
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
