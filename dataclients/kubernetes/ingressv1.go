package kubernetes

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
)

func setPathV1(m PathMode, r *eskip.Route, pathType, path string) {
	if path == "" {
		return
	}
	// see https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#httpingresspath-v1-networking-k8s-io
	switch pathType {
	case "Exact":
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: "Path",
			Args: []interface{}{path},
		})
	case "Prefix":
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: "PathSubtree",
			Args: []interface{}{path},
		})
	default:
		setPath(m, r, path)
	}
}

func convertPathRuleV1(
	state *clusterState,
	metadata *definitions.Metadata,
	host string,
	prule *definitions.PathRuleV1,
	pathMode PathMode,
	allowedExternalNames []*regexp.Regexp,
) (*eskip.Route, error) {

	ns := metadata.Namespace
	name := metadata.Name

	if prule.Backend == nil {
		return nil, fmt.Errorf("invalid path rule, missing backend in: %s/%s/%s", ns, name, host)
	}

	var (
		eps []string
		err error
		svc *service
	)

	var hostRegexp []string
	if host != "" {
		hostRegexp = []string{createHostRx(host)}
	}
	svcPort := prule.Backend.Service.Port
	svcName := prule.Backend.Service.Name

	svc, err = state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertPathRuleV1: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, err
	}

	servicePort, err := svc.getServicePortV1(svcPort)
	if err != nil {
		// service definition is wrong or no pods
		err = nil
		if len(eps) > 0 {
			// should never happen
			log.Errorf("convertPathRuleV1: Failed to find target port for service %s, but %d endpoints exist. Kubernetes has inconsistent data", svcName, len(eps))
		}
	} else if svc.Spec.Type == "ExternalName" {
		return externalNameRoute(ns, name, host, hostRegexp, svc, servicePort, allowedExternalNames)
	} else {
		protocol := "http"
		if p, ok := metadata.Annotations[skipperBackendProtocolAnnotationKey]; ok {
			protocol = p
		}

		eps = state.getEndpointsByService(ns, svcName, protocol, servicePort)
		log.Debugf("convertPathRuleV1: Found %d endpoints %s for %s", len(eps), servicePort, svcName)
	}
	if len(eps) == 0 {
		// add shunt route https://github.com/zalando/skipper/issues/1525
		log.Debugf("convertPathRuleV1: add shuntroute to return 502 for ingress %s/%s service %s with %d endpoints", ns, name, svcName, len(eps))
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			HostRegexps: hostRegexp,
		}

		setPathV1(pathMode, r, prule.PathType, prule.Path)
		setTraffic(r, svcName, prule.Backend.Traffic, prule.Backend.NoopCount)
		shuntRoute(r)
		return r, nil
	}

	log.Debugf("convertPathRuleV1: %d routes for %s/%s/%s", len(eps), ns, svcName, svcPort)
	if len(eps) == 1 {
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     eps[0],
			BackendType: eskip.NetworkBackend,
			HostRegexps: hostRegexp,
		}

		setPathV1(pathMode, r, prule.PathType, prule.Path)
		setTraffic(r, svcName, prule.Backend.Traffic, prule.Backend.NoopCount)
		return r, nil
	}

	r := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path, svcName),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(metadata),
		HostRegexps: hostRegexp,
	}
	setPathV1(pathMode, r, prule.PathType, prule.Path)
	setTraffic(r, svcName, prule.Backend.Traffic, prule.Backend.NoopCount)
	return r, nil
}

func (ing *ingress) addEndpointsRuleV1(ic ingressContext, host string, prule *definitions.PathRuleV1) error {
	meta := ic.ingressV1.Metadata
	endpointsRoute, err := convertPathRuleV1(
		ic.state,
		meta,
		host,
		prule,
		ic.pathMode,
		ing.allowedExternalNames,
	)
	if err != nil {
		// if the service is not found the route should be removed
		if err == errServiceNotFound || err == errResourceNotFound {
			return nil
		}

		// TODO: this error checking should not really be used, and the error handling of the ingress
		// problems should be refactored such that a single ingress's error doesn't block the
		// processing of the independent ingresses.
		if errors.Is(err, errNotAllowedExternalName) {
			log.Infof("Not allowed external name: %v", err)
			return nil
		}

		// Ingress status field does not support errors
		return fmt.Errorf("error while getting service: %v", err)
	}

	// safe prepend, see: https://play.golang.org/p/zg5aGKJpRyK
	filters := make([]*eskip.Filter, len(endpointsRoute.Filters)+len(ic.annotationFilters))
	copy(filters, ic.annotationFilters)
	copy(filters[len(ic.annotationFilters):], endpointsRoute.Filters)
	endpointsRoute.Filters = filters

	// add pre-configured default filters
	df, err := ic.defaultFilters.getNamed(meta.Namespace, prule.Backend.Service.Name)
	if err != nil {
		ic.logger.Errorf("Failed to retrieve default filters: %v.", err)
	} else {
		// it's safe to prepend, because type defaultFilters copies the slice during get()
		endpointsRoute.Filters = append(df, endpointsRoute.Filters...)
	}

	err = applyAnnotationPredicates(ic.pathMode, endpointsRoute, ic.annotationPredicate)
	if err != nil {
		ic.logger.Errorf("failed to apply annotation predicates: %v", err)
	}
	ic.addHostRoute(host, endpointsRoute)

	redirect := ic.redirect
	ewRangeMatch := false
	for _, s := range ing.eastWestRangeDomains {
		if strings.HasSuffix(host, s) {
			ewRangeMatch = true
			break
		}
	}
	if !(ewRangeMatch || strings.HasSuffix(host, ing.kubernetesEastWestDomain) && ing.kubernetesEastWestDomain != "") {
		switch {
		case redirect.ignore:
			// no redirect
		case redirect.enable:
			ic.addHostRoute(host, createIngressEnableHTTPSRedirect(endpointsRoute, redirect.code))
			redirect.setHost(host)
		case redirect.disable:
			ic.addHostRoute(host, createIngressDisableHTTPSRedirect(endpointsRoute))
			redirect.setHostDisabled(host)
		case redirect.defaultEnabled:
			ic.addHostRoute(host, createIngressEnableHTTPSRedirect(endpointsRoute, redirect.code))
			redirect.setHost(host)
		}
	}

	if ing.kubernetesEnableEastWest {
		ewRoute := createEastWestRouteIng(ing.kubernetesEastWestDomain, meta.Name, meta.Namespace, endpointsRoute)
		ewHost := fmt.Sprintf("%s.%s.%s", meta.Name, meta.Namespace, ing.kubernetesEastWestDomain)
		ic.addHostRoute(ewHost, ewRoute)
	}
	return nil
}

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

		if weight, ok := backendWeights[path.Backend.Service.Name]; ok {
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
			if weight, ok := backendWeights[path.Backend.Service.Name]; ok {
				// force a weight of 1.0 for the last backend with a non-zero weight to avoid rounding issues
				if sc.lastActive == path.Backend {
					path.Backend.Traffic = 1.0
					continue
				}

				path.Backend.Traffic = weight / sc.sum
				// subtract weight from the sum in order to
				// give subsequent backends a higher relative
				// weight.
				sc.sum -= weight

				// noops are required to make sure that routes are in order selected by
				// routing tree
				if sc.weightsCount > 2 {
					path.Backend.NoopCount = sc.weightsCount - 2
				}
				sc.weightsCount--
			} else if sc.sum == 0 && sc.count > 0 {
				path.Backend.Traffic = 1.0 / float64(sc.count)
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
		addExtraRoutes(ic, ru.Host, prule.Path, prule.PathType, ing.kubernetesEastWestDomain, ing.kubernetesEnableEastWest)
		if prule.Backend.Traffic > 0 {
			err := ing.addEndpointsRuleV1(ic, ru.Host, prule)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// addSpecIngressTLSV1 is used to add TLS Certificates to hostRoutes
func (ing *ingress) addSpecIngressTLSV1(ic ingressContext, ingtls *definitions.TLSV1) error {
	for host, _ := range ic.hostRoutes {
		for _, htls := range ingtls.Hosts {
			if htls == host {
				err := addHostTLSCerts(ic, host, ingtls.SecretName)
				if err != nil {
					return err
				}
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
		svcPort = i.Spec.DefaultBackend.Service.Port
	)

	svc, err := state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertDefaultBackendV1: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, false, err
	}

	servicePort, err := svc.getServicePortV1(svcPort)
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
			Id:          routeID(ns, name, "", "", ""),
			Backend:     eps[0],
			BackendType: eskip.NetworkBackend,
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
		ingressV1:           i,
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
	for _, ingtls := range i.Spec.IngressTLS {
		err := ing.addSpecIngressTLSV1(ic, ingtls)
		if err != nil {
			return nil, err
		}
	}
	return route, nil
}
