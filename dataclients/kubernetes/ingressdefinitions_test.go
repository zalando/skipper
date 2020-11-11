package kubernetes

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

func TestGetTargetPort(t *testing.T) {
	tests := []struct {
		name        string
		svc         *service
		svcPort     definitions.BackendPort
		expected    *definitions.BackendPort
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
			expected:    &definitions.BackendPort{Value: 80},
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
			expected:    nil,
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
