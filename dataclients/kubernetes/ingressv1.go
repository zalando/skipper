package kubernetes

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
)

func (ing *ingress) ingressV1Route(
	i *definitions.IngressV1Item,
	redirect *redirectInfo,
	state *clusterState,
	hostRoutes map[string][]*eskip.Route,
	df defaultFilters,
) (*eskip.Route, error) {
	if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" || i.Spec == nil {
		log.Error("invalid ingress item: missing Metadata or Spec")
		return nil, nil
	}
	logger := log.WithFields(log.Fields{
		"ingress": fmt.Sprintf("%s/%s", i.Metadata.Namespace, i.Metadata.Name),
	})
	redirect.initCurrent(i.Metadata)
	ic := ingressContext{
		state:               state,
		metadata:            i.Metadata,
		logger:              logger,
		annotationFilters:   annotationFilter(i.Metadata, logger),
		annotationPredicate: annotationPredicate(i.Metadata),
		extraRoutes:         extraRoutes(i.Metadata, logger),
		backendWeights:      backendWeights(i.Metadata, logger),
		pathMode:            pathMode(i.Metadata, ing.pathMode),
		redirect:            redirect,
		hostRoutes:          hostRoutes,
		defaultFilters:      df,
	}

	var route *eskip.Route

	// the usage of the default backend depends on what we want
	// we can generate a hostname out of it based on shared rules
	// and instructions in annotations, if there are no rules defined
	// this is a flaw in the ingress API design, because it is not on the hosts' level, but the spec
	// tells to match if no rule matches. This means that there is no matching rule on this ingress
	// and if there are multiple ingress items, then there is a race between them.
	if i.Spec.DefaultBackend != nil {
		if r, ok, err := ing.convertDefaultBackend(state, i.Spec.DefaultBackend, i.Metadata); ok {
			route = r
		} else if err != nil {
			ic.logger.Errorf("error while converting default backend: %v", err)
		}
	}
	for _, rule := range i.Spec.Rules {
		err := ing.addSpecRule(ic, rule)
		if err != nil {
			return nil, err
		}
	}
	return route, nil
}
