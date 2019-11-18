package kubernetes

import (
	"sort"

	log "github.com/sirupsen/logrus"
)

type clusterState struct {
	ingresses       []*ingressItem
	services        map[resourceId]*service
	endpoints       map[resourceId]*endpoint
	cachedEndpoints map[endpointId][]string
}

func (state *clusterState) getService(namespace, name string) (*service, error) {
	s, ok := state.services[newResourceId(namespace, name)]
	if !ok {
		return nil, errServiceNotFound
	}

	if s.Spec == nil {
		log.Debug("invalid service datagram, missing spec")
		return nil, errServiceNotFound
	}
	return s, nil
}

func (state *clusterState) getEndpoints(namespace, name, servicePort, targetPort string) ([]string, error) {
	epId := endpointId{
		resourceId:  newResourceId(namespace, name),
		servicePort: servicePort,
		targetPort:  targetPort,
	}

	if cached, ok := state.cachedEndpoints[epId]; ok {
		return cached, nil
	}

	ep, ok := state.endpoints[epId.resourceId]
	if !ok {
		return nil, errEndpointNotFound
	}

	if ep.Subsets == nil {
		return nil, errEndpointNotFound
	}

	targets := ep.targets(servicePort, targetPort)
	if len(targets) == 0 {
		return nil, errEndpointNotFound
	}
	sort.Strings(targets)
	state.cachedEndpoints[epId] = targets
	return targets, nil
}
