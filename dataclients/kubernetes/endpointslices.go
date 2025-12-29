package kubernetes

import (
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

const endpointSliceServiceNameLabel = "kubernetes.io/service-name"

// There are [1..N] Kubernetes endpointslices created for a single Kubernetes service.
// Kubernetes endpointslices of a given service can have duplicates with different states.
// Therefore Kubernetes endpointslices need to be de-duplicated before usage.
// The business object skipperEndpointSlice is a de-duplicated endpoint list that concatenates all endpointslices of a given service into one slice of skipperEndpointSlice.
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

func (eps *skipperEndpointSlice) getPort(protocol, pName string, pValue int) int {
	var port int
	for _, p := range eps.Ports {
		if protocol != "" && p.Protocol != protocol {
			continue
		}
		// https://pkg.go.dev/k8s.io/api/core/v1#ServicePort
		// Optional if only one ServicePort is defined on this service.
		// Therefore empty name match is fine.
		if p.Name == pName {
			port = p.Port
			break
		}
		if pValue != 0 && p.Port == pValue {
			port = pValue
			break
		}
	}

	return port
}
func (eps *skipperEndpointSlice) targetsByServicePort(protocol, scheme string, servicePort *servicePort) []string {
	var port int
	if servicePort.Name != "" {
		port = eps.getPort(protocol, servicePort.Name, servicePort.Port)
	} else if servicePort.TargetPort != nil {
		var ok bool
		port, ok = servicePort.TargetPort.Number()
		if !ok {
			port = eps.getPort(protocol, servicePort.Name, servicePort.Port)
		}
	} else {
		port = eps.getPort(protocol, servicePort.Name, servicePort.Port)
	}

	result := make([]string, 0, len(eps.Endpoints))
	for _, ep := range eps.Endpoints {
		result = append(result, formatEndpointString(ep.Address, scheme, port))
	}
	return result
}

func (eps *skipperEndpointSlice) targetsByServiceTarget(protocol, scheme string, serviceTarget *definitions.BackendPort) []string {
	pName, _ := serviceTarget.Value.(string)
	pValue, _ := serviceTarget.Value.(int)
	port := eps.getPort(protocol, pName, pValue)

	result := make([]string, 0, len(eps.Endpoints))
	for _, ep := range eps.Endpoints {
		result = append(result, formatEndpointString(ep.Address, scheme, port))
	}
	return result
}

func (eps *skipperEndpointSlice) addresses() []string {
	result := make([]string, 0, len(eps.Endpoints))
	for _, ep := range eps.Endpoints {
		result = append(result, ep.Address)
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
	svcName := eps.Meta.Labels[endpointSliceServiceNameLabel]
	namespace := eps.Meta.Namespace
	return newResourceID(namespace, svcName)
}

// EndpointSliceEndpoints is the single endpoint definition
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
	return ep.Conditions != nil && ep.Conditions.Terminating != nil && *ep.Conditions.Terminating
}

func (ep *EndpointSliceEndpoints) isReady() bool {
	if ep.isTerminating() {
		return false
	}
	// defaults to ready, see also https://github.com/kubernetes/kubernetes/blob/91aca10d5984313c1c5858979d4946ff9446615f/pkg/proxy/endpointslicecache.go#L137C39-L139
	// we ignore serving because of https://github.com/zalando/skipper/issues/2684
	return ep.Conditions.Ready == nil || *ep.Conditions.Ready
}
