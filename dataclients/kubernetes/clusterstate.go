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
	ingressesV1     []*definitions.IngressV1Item
	routeGroups     []*definitions.RouteGroupItem
	services        map[definitions.ResourceID]*service
	endpoints       map[definitions.ResourceID]*endpoint
	endpointSlices  map[definitions.ResourceID]*skipperEndpointSlice
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

// GetEndpointsByService returns the skipper endpoints for kubernetes endpoints or endpointslices.
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

	var targets []string
	eps, ok := state.endpointSlices[epID.ResourceID]
	if ok {
		targets = eps.targetsByServicePort("TCP", protocol, servicePort)
	} else {
		log.Warnf("GetEndpointsByService %s/%s - fallback to *deprecated* endpoint: %s/%s", namespace, name, epID.ResourceID.Namespace, epID.ResourceID.Name)
		ep, ok := state.endpoints[epID.ResourceID]
		if !ok {
			return nil
		}
		targets = ep.targetsByServicePort(protocol, servicePort)
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}

// GetEndpointsByName returns the skipper endpoints for kubernetes endpoints or endpointslices.
// This function works only correctly for endpointslices (and likely endpoints) with one port with the same protocol ("TCP", "UDP").
func (state *clusterState) GetEndpointsByName(namespace, name, protocol, backendProtocol string) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached
	}

	var targets []string
	eps, ok := state.endpointSlices[epID.ResourceID]
	if ok {
		targets = eps.targets(protocol, backendProtocol)
	} else {
		log.Warnf("GetEndpointsByName - fallback to *deprecated* endpoint: %s/%s", epID.ResourceID.Namespace, epID.ResourceID.Name)
		ep, ok := state.endpoints[epID.ResourceID]
		if !ok {
			return nil
		}
		targets = ep.targets(backendProtocol)
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets

}

// GetEndpointsByTarget returns the skipper endpoints for kubernetes endpoints or endpointslices.
func (state *clusterState) GetEndpointsByTarget(namespace, name, protocol, backendProtocol string, target *definitions.BackendPort) []string {
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

	var targets []string
	eps, ok := state.endpointSlices[epID.ResourceID]
	if ok {
		targets = eps.targetsByServiceTarget(protocol, backendProtocol, target)
	} else {
		log.Warnf("GetEndpointsByTarget - fallback to *deprecated* endpoint: %s/%s", epID.ResourceID.Namespace, epID.ResourceID.Name)
		ep, ok := state.endpoints[epID.ResourceID]
		if !ok {
			return nil
		}
		targets = ep.targetsByServiceTarget(backendProtocol, target)
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}
