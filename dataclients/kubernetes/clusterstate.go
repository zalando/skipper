package kubernetes

import (
	"fmt"
	"sort"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type clusterState struct {
	mu                   sync.Mutex
	ingressesV1          []*definitions.IngressV1Item
	routeGroups          []*definitions.RouteGroupItem
	services             map[definitions.ResourceID]*service
	endpoints            map[definitions.ResourceID]*endpoint
	endpointSlices       map[definitions.ResourceID]*skipperEndpointSlice
	secrets              map[definitions.ResourceID]*secret
	cachedEndpoints      map[endpointID][]string
	enableEndpointSlices bool
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
	if state.enableEndpointSlices {
		if eps, ok := state.endpointSlices[epID.ResourceID]; ok {
			targets = eps.targetsByServicePort("TCP", protocol, servicePort)
		} else {
			return nil
		}
	} else {
		if ep, ok := state.endpoints[epID.ResourceID]; ok {
			targets = ep.targetsByServicePort(protocol, servicePort)
		} else {
			return nil
		}
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}

// GetEndpointsByName returns the skipper endpoints for kubernetes endpoints or endpointslices.
// This function works only correctly for endpointslices (and likely endpoints) with one port with the same protocol ("TCP", "UDP").
func (state *clusterState) GetEndpointsByName(namespace, name, protocol, scheme string) []string {
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
	if state.enableEndpointSlices {
		if eps, ok := state.endpointSlices[epID.ResourceID]; ok {
			targets = eps.targets(protocol, scheme)
		} else {
			return nil
		}
	} else {
		if ep, ok := state.endpoints[epID.ResourceID]; ok {
			targets = ep.targets(scheme)
		} else {
			return nil
		}
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets

}

// GetEndpointsByTarget returns the skipper endpoints for kubernetes endpoints or endpointslices.
func (state *clusterState) GetEndpointsByTarget(namespace, name, protocol, scheme string, target *definitions.BackendPort) []string {
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
	if state.enableEndpointSlices {
		if eps, ok := state.endpointSlices[epID.ResourceID]; ok {
			targets = eps.targetsByServiceTarget(protocol, scheme, target)
		} else {
			return nil
		}
	} else {
		if ep, ok := state.endpoints[epID.ResourceID]; ok {
			targets = ep.targetsByServiceTarget(scheme, target)
		} else {
			return nil
		}
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}
