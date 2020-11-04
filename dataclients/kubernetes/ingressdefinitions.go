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

func (sp servicePort) matchingPort(svcPort definitions.BackendPort) bool {
	s := svcPort.String()
	spt := strconv.Itoa(sp.Port)
	return s != "" && (spt == s || sp.Name == s)
}

func (sp servicePort) String() string {
	return fmt.Sprintf("%s %d %s", sp.Name, sp.Port, sp.TargetPort)
}

type serviceSpec struct {
	Type         string         `json:"type"`
	ClusterIP    string         `json:"clusterIP"`
	ExternalName string         `json:"externalName"`
	Ports        []*servicePort `json:"ports"`
}

type service struct {
	Meta *definitions.Metadata `json:"Metadata"`
	Spec *serviceSpec          `json:"spec"`
}

type serviceList struct {
	Items []*service `json:"items"`
}

func (s service) getTargetPort(svcPort definitions.BackendPort) (string, error) {
	for _, sp := range s.Spec.Ports {
		if sp.matchingPort(svcPort) && sp.TargetPort != nil {
			return sp.TargetPort.String(), nil
		}
	}
	return "", fmt.Errorf("getTargetPort: target port not found %v given %v", s.Spec.Ports, svcPort)
}

func (s service) getTargetPortByValue(p int) (*definitions.BackendPort, bool) {
	for _, pi := range s.Spec.Ports {
		if pi.Port == p {
			return pi.TargetPort, true
		}
	}

	return nil, false
}

type endpoint struct {
	Meta    *definitions.Metadata `json:"Metadata"`
	Subsets []*subset             `json:"subsets"`
}

type endpointList struct {
	Items []*endpoint `json:"items"`
}

func formatEndpoint(a *address, p *port, protocol string) string {
	return fmt.Sprintf("%s://%s:%d", protocol, a.IP, p.Port)
}

// TODO(sszuecs): refactoring and replace it with clusterState.getEndpointsByTarget, but we need to support protocol to have feature parity with current ingress
// svcPortName is the truncated value of int or string from kubernetes service svcPort (should not be used here)
// svcPortTarget is the truncated value of int or string from kubernetes service targetPort
func (ep endpoint) targets(svcPortName, svcPortTarget, protocol string) []string {
	result := make([]string, 0)
	for _, s := range ep.Subsets {
		for _, port := range s.Ports {
			// TODO: we need to distinguish between the cases when there is no endpoints
			// and conversely, when there are endpoints and we just could not map the ports,
			// primarily when the service references the target port by name. Changes have
			// been started in this branch:
			//
			// https://github.com/zalando/skipper/tree/improvement/service-port-fallback-handling
			//
			if port.Name == svcPortName || port.Name == svcPortTarget || strconv.Itoa(port.Port) == svcPortTarget {
				for _, addr := range s.Addresses {
					result = append(result, formatEndpoint(addr, port, protocol))
				}
			}
		}
	}
	return result
}

func (ep endpoint) targetsByServiceTarget(serviceTarget *definitions.BackendPort) []string {
	portName, named := serviceTarget.Value.(string)
	portValue, byValue := serviceTarget.Value.(int)
	for _, s := range ep.Subsets {
		for _, p := range s.Ports {
			if named && p.Name != portName || byValue && p.Port != portValue {
				continue
			}

			var result []string
			for _, a := range s.Addresses {
				result = append(result, formatEndpoint(a, p, "http"))
			}

			return result
		}
	}

	return nil
}

type subset struct {
	Addresses []*address `json:"addresses"`
	Ports     []*port    `json:"ports"`
}

type address struct {
	IP   string `json:"ip"`
	Node string `json:"nodeName"`
}

type port struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

func newResourceID(namespace, name string) definitions.ResourceID {
	return definitions.ResourceID{Namespace: namespace, Name: name}
}

type endpointID struct {
	definitions.ResourceID
	servicePort string
	targetPort  string
}

type ClusterResource struct {
	Name string `json:"name"`
}

type ClusterResourceList struct {

	// Items, aka "resources".
	Items []*ClusterResource `json:"resources"`
}
