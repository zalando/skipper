package kubernetes

import (
	"fmt"
	"sort"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

type clusterState struct {
	ingresses       []*definitions.IngressItem
	ingressesV1     []*definitions.IngressV1Item
	routeGroups     []*definitions.RouteGroupItem
	services        map[definitions.ResourceID]*service
	endpoints       map[definitions.ResourceID]*endpoint
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

func (state *clusterState) getEndpointsByService(namespace, name, protocol string, servicePort *servicePort) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		protocol:   protocol,
		targetPort: servicePort.TargetPort.String(),
	}

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

func (state *clusterState) getEndpointsByTarget(namespace, name, protocol string, target *definitions.BackendPort) []string {
	epID := endpointID{
		ResourceID: newResourceID(namespace, name),
		protocol:   protocol,
		targetPort: target.String(),
	}

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
