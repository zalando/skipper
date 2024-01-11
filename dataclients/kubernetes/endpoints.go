package kubernetes

import (
	"net"
	"strconv"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type endpointID struct {
	definitions.ResourceID
	TargetPort string
	Protocol   string
}

type endpoint struct {
	Meta    *definitions.Metadata `json:"metadata"`
	Subsets []*subset             `json:"subsets"`
}

type endpointList struct {
	Items []*endpoint `json:"items"`
}

func formatEndpointString(ip, protocol string, port int) string {
	return protocol + "://" + net.JoinHostPort(ip, strconv.Itoa(port))
}

func formatEndpoint(a *address, p *port, protocol string) string {
	return formatEndpointString(a.IP, protocol, p.Port)
}

func formatEndpointsForSubsetAddresses(addresses []*address, port *port, protocol string) []string {
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, formatEndpoint(address, port, protocol))
	}
	return result
}

func (ep *endpoint) targetsByServicePort(protocol string, servicePort *servicePort) []string {
	for _, s := range ep.Subsets {
		// If only one port exists in the endpoint, use it
		if len(s.Ports) == 1 {
			return formatEndpointsForSubsetAddresses(s.Addresses, s.Ports[0], protocol)
		}

		// Otherwise match port by name
		for _, p := range s.Ports {
			if p.Name != servicePort.Name {
				continue
			}

			return formatEndpointsForSubsetAddresses(s.Addresses, p, protocol)
		}
	}

	return nil
}

func (ep *endpoint) targetsByServiceTarget(protocol string, serviceTarget *definitions.BackendPort) []string {
	portName, named := serviceTarget.Value.(string)
	portValue, byValue := serviceTarget.Value.(int)
	for _, s := range ep.Subsets {
		for _, p := range s.Ports {
			if named && p.Name != portName || byValue && p.Port != portValue {
				continue
			}

			var result []string
			for _, a := range s.Addresses {
				result = append(result, formatEndpoint(a, p, protocol))
			}

			return result
		}
	}

	return nil
}

func (ep *endpoint) targets(protocol string) []string {
	result := make([]string, 0)
	for _, s := range ep.Subsets {
		for _, p := range s.Ports {
			for _, a := range s.Addresses {
				result = append(result, formatEndpoint(a, p, protocol))
			}
		}
	}

	return result
}

type subset struct {
	Addresses []*address `json:"addresses"`
	Ports     []*port    `json:"ports"`
}

type address struct {
	IP        string           `json:"ip"`
	NodeName  string           `json:"nodeName"`
	TargetRef *objectReference `json:"targetRef"`
}

type port struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}
