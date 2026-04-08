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
	cachedEndpointSlices map[endpointID][]skipperEndpoint
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
		return nil, fmt.Errorf("%s/%s: %w", namespace, name, errServiceNotFound)
	}

	return s, nil
}

// GetEndpointsByService returns the skipper endpoints for kubernetes endpoints.
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
	var ep *endpoint
	ep, ok := state.endpoints[epID.ResourceID]
	if !ok {
		return nil
	}

	targets = ep.targetsByServicePort(protocol, servicePort)

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}

// GetEndpointSlicesByService returns the skipper endpointslices for kubernetes endpointslices.
func (state *clusterState) GetEndpointSlicesByService(zone, namespace, name, protocol string, servicePort *servicePort) []skipperEndpoint {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
		TargetPort: servicePort.TargetPort.String(),
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	var targets []skipperEndpoint
	if cached, ok := state.cachedEndpointSlices[epID]; ok {
		targets = cached
	} else {
		if eps, ok := state.endpointSlices[epID.ResourceID]; ok {
			targets = eps.targetsByServicePort("TCP", protocol, servicePort)
		} else {
			return nil
		}

		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Address < targets[j].Address
		})

		state.cachedEndpointSlices[epID] = targets
	}

	return filterByZone(zone, targets)
}

// getEndpointAddresses returns the list of all addresses for the given service using endpoints or endpointslices.
func (state *clusterState) getEndpointAddresses(zone, namespace, name string) []string {
	rID := newResourceID(namespace, name)

	state.mu.Lock()
	defer state.mu.Unlock()

	var addresses []string
	if state.enableEndpointSlices {
		if eps, ok := state.endpointSlices[rID]; ok {
			if zone != "" {
				addresses = eps.addressesByZone(zone)
			} else {
				addresses = eps.addresses()
			}
		} else {
			return nil
		}
	} else {
		if ep, ok := state.endpoints[rID]; ok {
			addresses = ep.addresses()
		} else {
			return nil
		}
	}
	sort.Strings(addresses)

	return addresses
}

// GetEndpointsByTarget returns the skipper endpoints for kubernetes endpoints.
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
	var ep *endpoint
	ep, ok := state.endpoints[epID.ResourceID]
	if !ok {
		return nil
	}

	targets = ep.targetsByServiceTarget(scheme, target)

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}

// GetEndpointSlicesByTarget returns the skipper endpointslices for kubernetes endpointslices.
func (state *clusterState) GetEndpointSlicesByTarget(zone, namespace, name, protocol, scheme string, target *definitions.BackendPort) []skipperEndpoint {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		Protocol:   protocol,
		TargetPort: target.String(),
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	var targets []skipperEndpoint
	if cached, ok := state.cachedEndpointSlices[epID]; ok {
		targets = cached
	} else {
		if eps, ok := state.endpointSlices[epID.ResourceID]; ok {
			targets = eps.targetsByServiceTarget(protocol, scheme, target)
		} else {
			return nil
		}

		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Address < targets[j].Address
		})
		state.cachedEndpointSlices[epID] = targets
	}

	return filterByZone(zone, targets)
}

func filterByZone(zone string, targets []skipperEndpoint) []skipperEndpoint {
	if zone != "" {
		var zoneTargets []skipperEndpoint
		for _, target := range targets {
			if target.Zone == zone {
				zoneTargets = append(zoneTargets, target)
			}
		}
		if len(zoneTargets) >= minEndpointsByZone {
			return zoneTargets
		}
	}
	return targets
}
