package kubernetes

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"
)

type clusterState struct {
	ingresses       []*ingressItem
	routeGroups     []*routeGroupItem
	services        map[resourceID]*service
	endpoints       map[resourceID]*endpoint
	cachedEndpoints map[endpointID][]string
}

func (state *clusterState) getService(namespace, name string) (*service, error) {
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
	s, ok := state.services[newResourceID(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("service not found: %s/%s", namespace, name)
	}

	return s, nil
}

// Always returns one or more endpoints or an error. If there are endpoints with one or more subsets, but
// the endpoint port could not be mapped, it returns errEndpointMappingFailed as the error.
func (state *clusterState) getEndpoints(namespace, name, servicePort, targetPort string) ([]string, error) {
	epID := endpointID{
		resourceID:  newResourceID(namespace, name),
		servicePort: servicePort,
		targetPort:  targetPort,
	}

	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached, nil
	}

	ep, ok := state.endpoints[epID.resourceID]
	if !ok {
		return nil, errEndpointNotFound
	}

	if len(ep.Subsets) == 0 {
		return nil, errEndpointNotFound
	}

	targets := ep.targets(servicePort, targetPort)
	if len(targets) == 0 {
		return nil, errEndpointMappingFailed
	}

	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets, nil
}

func (state *clusterState) getEndpointsByTarget(namespace, name string, target *backendPort) []string {
	epID := endpointID{
		resourceID: newResourceID(namespace, name),
		targetPort: target.String(),
	}

	if cached, ok := state.cachedEndpoints[epID]; ok {
		return cached
	}

	ep, ok := state.endpoints[epID.resourceID]
	if !ok {
		return nil
	}

	targets := ep.targetsByServiceTarget(target)
	sort.Strings(targets)
	state.cachedEndpoints[epID] = targets
	return targets
}
