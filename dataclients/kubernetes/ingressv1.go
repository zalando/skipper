package kubernetes

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
)

// computeBackendWeightsV1 computes and sets the backend traffic weights on the
// rule backends.
// The traffic is calculated based on the following rules:
//
// * if no weight is defined for a backend it will get weight 0.
// * if no weights are specified for all backends of a path, then traffic will
//   be distributed equally.
//
// Each traffic weight is relative to the number of backends per path. If there
// are multiple backends per path the weight will be relative to the number of
// remaining backends for the path e.g. if the weight is specified as
//
//      backend-1: 0.2
//      backend-2: 0.6
//      backend-3: 0.2
//
// then the weight will be calculated to:
//
//      backend-1: 0.2
//      backend-2: 0.75
//      backend-3: 1.0
//
// where for a weight of 1.0 no Traffic predicate will be generated.
func computeBackendWeightsV1(backendWeights map[string]float64, rule *definitions.RuleV1) {
	type pathInfo struct {
		sum          float64
		lastActive   *definitions.BackendV1
		count        int
		weightsCount int
	}

	// get backend weight sum and count of backends for all paths
	pathInfos := make(map[string]*pathInfo)
	for _, path := range rule.Http.Paths {
		sc, ok := pathInfos[path.Path]
		if !ok {
			sc = &pathInfo{}
			pathInfos[path.Path] = sc
		}

		if weight, ok := backendWeights[path.Backend.GetServiceName()]; ok {
			sc.sum += weight
			if weight > 0 {
				sc.lastActive = path.Backend
				sc.weightsCount++
			}
		} else {
			sc.count++
		}
	}

	// calculate traffic weight for each backend
	for _, path := range rule.Http.Paths {
		if sc, ok := pathInfos[path.Path]; ok {
			if weight, ok := backendWeights[path.Backend.GetServiceName()]; ok {
				// force a weight of 1.0 for the last backend with a non-zero weight to avoid rounding issues
				if sc.lastActive == path.Backend {
					path.Backend.GetTraffic().Weight = 1.0
					continue
				}

				path.Backend.GetTraffic().Weight = weight / sc.sum
				// subtract weight from the sum in order to
				// give subsequent backends a higher relative
				// weight.
				sc.sum -= weight

				// noops are required to make sure that routes are in order selected by
				// routing tree
				if sc.weightsCount > 2 {
					path.Backend.GetTraffic().NoopCount = sc.weightsCount - 2
				}
				sc.weightsCount--
			} else if sc.sum == 0 && sc.count > 0 {
				path.Backend.GetTraffic().Weight = 1.0 / float64(sc.count)
			}
			// reduce count by one in order to give subsequent
			// backends for the path a higher relative weight.
			sc.count--
		}
	}
}

// TODO: default filters not applied to 'extra' routes from the custom route annotations. Is it on purpose?
// https://github.com/zalando/skipper/issues/1287
func (ing *ingress) addSpecRuleV1(ic ingressContext, ru *definitions.RuleV1) error {
	if ru.Http == nil {
		ic.logger.Warn("invalid ingress item: rule missing http definitions")
		return nil
	}
	// update Traffic field for each backend
	computeBackendWeightsV1(ic.backendWeights, ru)
	for _, prule := range ru.Http.Paths {
		addExtraRoutes(ic, ru.Host, prule, ing.kubernetesEastWestDomain, ing.kubernetesEnableEastWest)
		if prule.Backend.GetTraffic().Weight > 0 {
			err := ing.addEndpointsRule(ic, ru.Host, prule)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// converts the default backend if any
func (ing *ingress) convertDefaultBackendV1(
	state *clusterState,
	i *definitions.IngressV1Item,
) (*eskip.Route, bool, error) {
	// the usage of the default backend depends on what we want
	// we can generate a hostname out of it based on shared rules
	// and instructions in annotations, if there are no rules defined

	// this is a flaw in the ingress API design, because it is not on the hosts' level, but the spec
	// tells to match if no rule matches. This means that there is no matching rule on this ingress
	// and if there are multiple ingress items, then there is a race between them.
	if i.Spec.DefaultBackend == nil {
		return nil, false, nil
	}

	var (
		eps     []string
		err     error
		ns      = i.Metadata.Namespace
		name    = i.Metadata.Name
		svcName = i.Spec.DefaultBackend.Service.Name
		svcPort = i.Spec.DefaultBackend.Service.Port.String()
	)

	svc, err := state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertDefaultBackendV1: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, false, err
	}

	servicePort, err := svc.getServicePort(svcPort)
	if err != nil {
		log.Errorf("convertDefaultBackendV1: Failed to find target port %v, %s, for ingress %s/%s and service %s add shuntroute: %v", svc.Spec.Ports, svcPort, ns, name, svcName, err)
		err = nil
	} else if svc.Spec.Type == "ExternalName" {
		r, err := externalNameRoute(ns, name, "default", nil, svc, servicePort, ing.allowedExternalNames)
		return r, err == nil, err
	} else {
		log.Debugf("convertDefaultBackendV1: Found target port %v, for service %s", servicePort.TargetPort, svcName)
		protocol := "http"
		if p, ok := i.Metadata.Annotations[skipperBackendProtocolAnnotationKey]; ok {
			protocol = p
		}

		eps = state.getEndpointsByService(
			ns,
			svcName,
			protocol,
			servicePort,
		)
		log.Debugf("convertDefaultBackendV1: Found %d endpoints for %s: %v", len(eps), svcName, err)
	}

	if len(eps) == 0 {
		// add shunt route https://github.com/zalando/skipper/issues/1525
		log.Debugf("convertDefaultBackendV1: add shuntroute to return 502 for ingress %s/%s service %s with %d endpoints", ns, name, svcName, len(eps))
		r := &eskip.Route{
			Id: routeID(ns, name, "", "", ""),
		}
		shuntRoute(r)
		return r, true, nil
	} else if len(eps) == 1 {
		return &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: eps[0],
		}, true, nil
	}

	return &eskip.Route{
		Id:          routeID(ns, name, "", "", ""),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(i.Metadata),
	}, true, nil
}

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
	if r, ok, err := ing.convertDefaultBackendV1(state, i); ok {
		route = r
	} else if err != nil {
		ic.logger.Errorf("error while converting default backend: %v", err)
	}
	for _, rule := range i.Spec.Rules {
		err := ing.addSpecRuleV1(ic, rule)
		if err != nil {
			return nil, err
		}
	}
	return route, nil
}
