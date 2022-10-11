package kubernetes

import (
	"fmt"
	"sort"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type clusterState struct {
	mu              sync.Mutex
	ingresses       []*definitions.IngressItem
	ingressesV1     []*definitions.IngressV1Item
	routeGroups     []*definitions.RouteGroupItem
	services        map[definitions.ResourceID]*service
	endpoints       map[definitions.ResourceID]*endpoint
	secrets         map[definitions.ResourceID]*secret
	cachedEndpoints map[endpointID][]string
}

func (state *clusterState) getService(namespace, name string) (*service, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	s, ok := state.services[newResourceID(namespace, name)]
	if !ok {
		return nil, errServiceNotFound
	}

	if s.Spec == nil {
		log.Debug("invalid service datagram, missing spec")
		return nil, errServiceNotFound
	}

	return s, nil
}

func (state *clusterState) getServiceRG(namespace, name string) (*service, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	s, ok := state.services[newResourceID(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("service not found: %s/%s", namespace, name)
	}

	return s, nil
}

func (state *clusterState) GetEndpointsByService(namespace, name, protocol string, servicePort *servicePort) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
		TargetPort: servicePort.TargetPort.String(),
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached
	}

	ep, ok := state.endpoints[epID.ResourceID]
	if !ok {
		return nil
	}

	targets := ep.targetsByServicePort(protocol, servicePort)
	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}

func (state *clusterState) GetEndpointsByName(namespace, name, protocol string) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached
	}

	ep, ok := state.endpoints[epID.ResourceID]
	if !ok {
		return nil
	}

	targets := ep.targets(protocol)
	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets

}

func (state *clusterState) GetEndpointsByTarget(namespace, name, protocol string, target *definitions.BackendPort) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
		TargetPort: target.String(),
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached
	}

	ep, ok := state.endpoints[epID.ResourceID]
	if !ok {
		return nil
	}

	targets := ep.targetsByServiceTarget(protocol, target)
	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}
