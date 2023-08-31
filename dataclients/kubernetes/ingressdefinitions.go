package kubernetes

import (
	"fmt"
	"net"
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

func (sp servicePort) matchingPortV1(svcPort definitions.BackendPortV1) bool {
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

func (s service) getServicePortV1(port definitions.BackendPortV1) (*servicePort, error) {
	for _, sp := range s.Spec.Ports {
		if sp.matchingPortV1(port) && sp.TargetPort != nil {
			return sp, nil
		}
	}
	return nil, fmt.Errorf("getServicePortV1: service port not found %v given %v", s.Spec.Ports, port)
}

func (s service) getTargetPortByValue(p int) (*definitions.BackendPort, bool) {
	for _, pi := range s.Spec.Ports {
		if pi.Port == p {
			return pi.TargetPort, true
		}
	}

	return nil, false
}

// There are [1..N] Kubernetes endpointslices created for a single Kubernetes service.
// Kubernetes endpointslices of a given service can have duplicates with different states.
// Therefore Kubernetes endpointslices need to be de-duplicated before usage.
// The business object skipperEndpointSlice is a de-duplicated endpoint list that concats all endpointslices of a given service into one slice of skipperEndpointSlice.
type skipperEndpointSlice struct {
	Meta      *definitions.Metadata
	Endpoints []*skipperEndpoint
	Ports     []*endpointSlicePort
}

// Conditions have to be evaluated before creation
type skipperEndpoint struct {
	Address string
	Zone    string
}

func (eps skipperEndpointSlice) getPort(protocol, pName string, pValue int) int {
	var port int

	for _, p := range eps.Ports {
		if p.Protocol != protocol {
			continue
		}
		if p.Name == pName {
			port = p.Port
			break
		}
		if p.Port == pValue {
			port = pValue
			break
		}
	}

	return port
}
func (eps skipperEndpointSlice) targetsByServicePort(protocol, backendProtocol string, servicePort *servicePort) []string {
	port := eps.getPort(protocol, servicePort.Name, servicePort.Port)

	var result []string
	for _, ep := range eps.Endpoints {
		result = append(result, formatEndpointString(ep.Address, backendProtocol, port))
	}

	return result
}

func (eps skipperEndpointSlice) targetsByServiceTarget(protocol, backendProtocol string, serviceTarget *definitions.BackendPort) []string {
	pName := serviceTarget.Value.(string)
	pValue := serviceTarget.Value.(int)
	port := eps.getPort(protocol, pName, pValue)

	var result []string
	for _, ep := range eps.Endpoints {
		result = append(result, formatEndpointString(ep.Address, backendProtocol, port))
	}

	return result
}

func (eps skipperEndpointSlice) targets(protocol, backendProtocol string) []string {
	result := make([]string, 0)

	var port int
	for _, p := range eps.Ports {
		if p.Protocol == protocol {
			port = p.Port
			break
		}
	}
	for _, ep := range eps.Endpoints {
		result = append(result, formatEndpointString(ep.Address, backendProtocol, port))
	}

	return result
}

type endpointSliceList struct {
	Meta  *definitions.Metadata
	Items []*endpointSlice `json:"items"`
}

// see https://kubernetes.io/docs/reference/kubernetes-api/service-resources/endpoint-slice-v1/#EndpointSlice
type endpointSlice struct {
	Meta        *definitions.Metadata     `json:"metadata"`
	AddressType string                    `json:"addressType"` // "IPv4"
	Endpoints   []*EndpointSliceEndpoints `json:"endpoints"`
	Ports       []*endpointSlicePort      `json:"ports"` // contains all ports like 9999/9911
}

// ToResourceID returns the same string for a group endpointlisces created for the same svc
func (eps *endpointSlice) ToResourceID() definitions.ResourceID {
	svcName := eps.Meta.Labels["kubernetes.io/service-name"]
	namespace := eps.Meta.Namespace
	return newResourceID(namespace, svcName)
}

// TODO(sszuecs): name TBD, endpoints would be ambiguous because of the other endpoint object
type EndpointSliceEndpoints struct {
	// Addresses [1..100] of the same AddressType, see also https://github.com/kubernetes/kubernetes/issues/106267
	// Basically it always has only one in our case and likely makes no sense to use more than one.
	// Pick first or one at random are possible, but skipper will pick the first.
	// If you need something else please create an issue https://github.com/zalando/skipper/issues/new/choose
	Addresses []string `json:"addresses"` // [ "10.2.13.9" ]
	// Conditions are used for deciding to drop out of load balancer or fade into the load balancer.
	Conditions *endpointsliceCondition `json:"conditions"`
	// Zone is used for zone aware traffic routing, please see also
	// https://kubernetes.io/docs/concepts/services-networking/topology-aware-routing/#constraints
	// https://kubernetes.io/docs/concepts/services-networking/topology-aware-routing/#safeguards
	// Zone aware routing will be available if https://github.com/zalando/skipper/issues/1446 is closed.
	Zone string `json:"zone"` // "eu-central-1c"
}

type endpointsliceCondition struct {
	Ready       *bool `json:"ready"`       // ready endpoint -> put into endpoints unless terminating
	Serving     *bool `json:"serving"`     // serving endpoint
	Terminating *bool `json:"terminating"` // termiating pod -> drop out of endpoints
}

type endpointSlicePort struct {
	Name     string `json:"name"`     // "http"
	Port     int    `json:"port"`     // 8080
	Protocol string `json:"protocol"` // "TCP"
	// AppProtocol is not used, but would make it possible to optimize H2C and websocket connections
	AppProtocol string `json:"appProtocol"` // "kubernetes.io/h2c", "kubernetes.io/ws", "kubernetes.io/wss"
}

func (ep *EndpointSliceEndpoints) isTerminating() bool {
	// see also https://github.com/kubernetes/kubernetes/blob/91aca10d5984313c1c5858979d4946ff9446615f/pkg/proxy/endpointslicecache.go#L137C39-L139
	return ep.Conditions.Terminating != nil && *ep.Conditions.Terminating
}

func (ep *EndpointSliceEndpoints) isReady() bool {
	if ep.isTerminating() {
		return false
	}
	// defaults to ready, see also https://github.com/kubernetes/kubernetes/blob/91aca10d5984313c1c5858979d4946ff9446615f/pkg/proxy/endpointslicecache.go#L137C39-L139
	return ep.Conditions.Ready == nil || *ep.Conditions.Ready || ep.Conditions.Serving == nil || *ep.Conditions.Serving
}

type endpoint struct {
	Meta    *definitions.Metadata `json:"Metadata"`
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
	var result []string
	for _, address := range addresses {
		result = append(result, formatEndpoint(address, port, protocol))
	}

	return result

}

func (ep endpoint) targetsByServicePort(protocol string, servicePort *servicePort) []string {
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

func (ep endpoint) targetsByServiceTarget(protocol string, serviceTarget *definitions.BackendPort) []string {
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

func (ep endpoint) targets(protocol string) []string {
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
	TargetPort string
	Protocol   string
}

type ClusterResource struct {
	Name string `json:"name"`
}

type ClusterResourceList struct {

	// Items, aka "resources".
	Items []*ClusterResource `json:"resources"`
}

type secret struct {
	Metadata *definitions.Metadata `json:"metadata"`
	Type     string                `json:"type"`
	Data     map[string]string     `json:"data"`
}

type secretList struct {
	Items []*secret `json:"items"`
}
