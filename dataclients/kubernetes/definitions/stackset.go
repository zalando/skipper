package definitions

import "k8s.io/apimachinery/pkg/util/intstr"

// https://github.com/zalando-incubator/stackset-controller/blob/master/pkg/apis/zalando.org/v1/types.go#L372-L383
type ActualTraffic struct {
	StackName   string             `json:"stackName"`
	ServiceName string             `json:"serviceName"`
	ServicePort intstr.IntOrString `json:"servicePort"`

	// +kubebuilder:validation:Format=float
	// +kubebuilder:validation:Type=number
	Weight float64 `json:"weight"`
}
