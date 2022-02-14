package kubernetes

import (
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

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
