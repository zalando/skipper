package kubernetes

import (
	"fmt"
	"strconv"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type servicePort struct {
	Name       string                   `json:"name"`
	Port       int                      `json:"port"`
	TargetPort *definitions.BackendPort `json:"targetPort"` // string or int
}

func (sp *servicePort) matchingPort(svcPort definitions.BackendPort) bool {
	s := svcPort.String()
	spt := strconv.Itoa(sp.Port)
	return s != "" && (spt == s || sp.Name == s)
}

func (sp *servicePort) matchingPortV1(svcPort definitions.BackendPortV1) bool {
	s := svcPort.String()
	spt := strconv.Itoa(sp.Port)
	return s != "" && (spt == s || sp.Name == s)
}

func (sp *servicePort) String() string {
	return fmt.Sprintf("%s %d %s", sp.Name, sp.Port, sp.TargetPort)
}

type serviceSpec struct {
	Type         string         `json:"type"`
	ClusterIP    string         `json:"clusterIP"`
	ExternalName string         `json:"externalName"`
	ExternalIPs  []string       `json:"externalIPs"`
	Ports        []*servicePort `json:"ports"`
}

type serviceStatus struct {
	LoadBalancer serviceLoadBalancerStatus `json:"loadBalancer"`
}

type serviceLoadBalancerStatus struct {
	Ingress []serviceLoadBalancerIngress `json:"ingress"`
}

type serviceLoadBalancerIngress struct {
	IP       string `json:"ip,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type service struct {
	Meta   *definitions.Metadata `json:"Metadata"`
	Spec   *serviceSpec          `json:"spec"`
	Status *serviceStatus        `json:"status"`
}

type serviceList struct {
	Items []*service `json:"items"`
}

func (s *service) getServicePortV1(port definitions.BackendPortV1) (*servicePort, error) {
	for _, sp := range s.Spec.Ports {
		if sp.matchingPortV1(port) && sp.TargetPort != nil {
			return sp, nil
		}
	}
	return nil, fmt.Errorf("getServicePortV1: service port not found %v given %v", s.Spec.Ports, port)
}

func (s *service) getTargetPortByValue(p int) (*definitions.BackendPort, bool) {
	for _, pi := range s.Spec.Ports {
		if pi.Port == p {
			return pi.TargetPort, true
		}
	}

	return nil, false
}
