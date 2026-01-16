package kubernetes

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/secrets/certregistry"
)

type weightedIngressBackend struct {
	name   string
	weight float64
}

var _ definitions.WeightedBackend = &weightedIngressBackend{}

func (b *weightedIngressBackend) GetName() string    { return b.name }
func (b *weightedIngressBackend) GetWeight() float64 { return b.weight }

func setPathOld(m PathMode, r *eskip.Route, p string) {
	switch m {
	case PathPrefix:
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: "PathSubtree",
			Args: []interface{}{p},
		})
	case PathRegexp:
		r.PathRegexps = []string{p}
	default:
		if p == "/" {
			r.PathRegexps = []string{"^/"}
		} else {
			r.PathRegexps = []string{"^(" + p + ")"}
		}
	}
}

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
		setPathOld(m, r, path)
	}
}

func convertPathRuleV1(
	ic *ingressContext,
	host string,
	prule *definitions.PathRuleV1,
	traffic backendTraffic,
	allowedExternalNames []*regexp.Regexp,
	forceKubernetesService bool,
	defaultLoadBalancerAlgorithm string,
) (*eskip.Route, error) {

	state := ic.state
	metadata := ic.ingressV1.Metadata
	pathMode := ic.pathMode
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
		ic.logger.Errorf("Failed to get service %s, %s", svcName, svcPort)
		return nil, err
	}

	servicePort, err := svc.getServicePortV1(svcPort)
	if err != nil {
		// service definition is wrong or no pods
		err = nil
		if len(eps) > 0 {
			// should never happen
			ic.logger.Errorf("Failed to find target port for service %s, but %d endpoints exist", svcName, len(eps))
		}
	} else if svc.Spec.Type == "ExternalName" {
		return externalNameRoute(ns, name, host, hostRegexp, svc, servicePort, allowedExternalNames)
	} else if forceKubernetesService {
		eps = []string{serviceNameBackend(svcName, ns, servicePort)}
	} else {
		protocol := "http"
		if p, ok := metadata.Annotations[skipperBackendProtocolAnnotationKey]; ok {
			protocol = p
		}

		eps = state.GetEndpointsByService(ic.zone, ns, svcName, protocol, servicePort)
	}
	if len(eps) == 0 {
		ic.logger.Tracef("Target endpoints not found, shuntroute for %s:%s", svcName, svcPort)

		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			HostRegexps: hostRegexp,
		}

		setPathV1(pathMode, r, prule.PathType, prule.Path)
		traffic.apply(r)
		shuntRoute(r)
		return r, nil
	}

	ic.logger.Tracef("Found %d endpoints for %s:%s", len(eps), svcName, svcPort)
	if len(eps) == 1 {
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     eps[0],
			BackendType: eskip.NetworkBackend,
			HostRegexps: hostRegexp,
		}

		setPathV1(pathMode, r, prule.PathType, prule.Path)
		traffic.apply(r)
		return r, nil
	}

	r := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path, svcName),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(metadata, defaultLoadBalancerAlgorithm),
		HostRegexps: hostRegexp,
	}
	setPathV1(pathMode, r, prule.PathType, prule.Path)
	traffic.apply(r)
	return r, nil
}

func (ing *ingress) addEndpointsRuleV1(ic *ingressContext, host string, prule *definitions.PathRuleV1, traffic backendTraffic) error {
	meta := ic.ingressV1.Metadata
	endpointsRoute, err := convertPathRuleV1(
		ic,
		host,
		prule,
		traffic,
		ing.allowedExternalNames,
		ing.forceKubernetesService,
		ing.defaultLoadBalancerAlgorithm,
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
			ic.logger.Infof("Not allowed external name: %v", err)
			return nil
		}

		// Ingress status field does not support errors
		return fmt.Errorf("error while getting service: %w", err)
	}

	if endpointsRoute.BackendType != eskip.ShuntBackend {
		// safe prepend, see: https://play.golang.org/p/zg5aGKJpRyK
		filters := make([]*eskip.Filter, len(endpointsRoute.Filters)+len(ic.annotationFilters))
		copy(filters, ic.annotationFilters)
		copy(filters[len(ic.annotationFilters):], endpointsRoute.Filters)
		endpointsRoute.Filters = filters
	}

	// add pre-configured default filters
	df, err := ic.defaultFilters.getNamed(meta.Namespace, prule.Backend.Service.Name)
	if err != nil {
		ic.logger.Errorf("Failed to retrieve default filters: %v", err)
	} else {
		// it's safe to prepend, because type defaultFilters copies the slice during get()
		endpointsRoute.Filters = append(df, endpointsRoute.Filters...)
	}

	err = applyAnnotationPredicates(ic.pathMode, endpointsRoute, ic.annotationPredicate)
	if err != nil {
		ic.logger.Errorf("Failed to apply annotation predicates: %v", err)
	}

	ic.addHostRoute(host, endpointsRoute)

	redirect := ic.redirect
	ewRangeMatch := isEastWestHost(host, ing.eastWestRangeDomains)
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

		appendAnnotationPredicates(ing.kubernetesAnnotationPredicates, meta.Annotations, endpointsRoute)
		appendAnnotationFilters(ing.kubernetesAnnotationFiltersAppend, meta.Annotations, endpointsRoute)
	} else {
		appendAnnotationPredicates(ing.kubernetesEastWestRangeAnnotationPredicates, meta.Annotations, endpointsRoute)
		appendAnnotationFilters(ing.kubernetesEastWestRangeAnnotationFiltersAppend, meta.Annotations, endpointsRoute)
	}

	if ing.kubernetesEnableEastWest {
		ewRoute := createEastWestRouteIng(ing.kubernetesEastWestDomain, meta.Name, meta.Namespace, endpointsRoute)
		ewHost := fmt.Sprintf("%s.%s.%s", meta.Name, meta.Namespace, ing.kubernetesEastWestDomain)
		ic.addHostRoute(ewHost, ewRoute)
	}
	return nil
}

// computeBackendWeightsV1 computes backend traffic weights for the rule backends grouped by path rule.
func computeBackendWeightsV1(calculateTraffic func([]*weightedIngressBackend) map[string]backendTraffic, backendWeights map[string]float64, rule *definitions.RuleV1) map[*definitions.PathRuleV1]backendTraffic {
	backendsPerPath := make(map[string][]*weightedIngressBackend)
	for _, prule := range rule.Http.Paths {
		b := &weightedIngressBackend{
			name:   prule.Backend.Service.Name,
			weight: backendWeights[prule.Backend.Service.Name],
		}
		backendsPerPath[prule.Path] = append(backendsPerPath[prule.Path], b)
	}

	trafficPerPath := make(map[string]map[string]backendTraffic, len(backendsPerPath))
	for path, b := range backendsPerPath {
		trafficPerPath[path] = calculateTraffic(b)
	}

	trafficPerPathRule := make(map[*definitions.PathRuleV1]backendTraffic)
	for _, prule := range rule.Http.Paths {
		trafficPerPathRule[prule] = trafficPerPath[prule.Path][prule.Backend.Service.Name]
	}

	return trafficPerPathRule
}

// TODO: default filters not applied to 'extra' routes from the custom route annotations. Is it on purpose?
// https://github.com/zalando/skipper/issues/1287
func (ing *ingress) addSpecRuleV1(ic *ingressContext, ru *definitions.RuleV1) error {
	if ru.Http == nil {
		ic.logger.Infof("Skipping rule without http definition")
		return nil
	}

	trafficPerPathRule := computeBackendWeightsV1(ic.calculateTraffic, ic.backendWeights, ru)

	for _, prule := range ru.Http.Paths {
		ing.addExtraRoutes(ic, ru.Host, prule.Path, prule.PathType)
		if trafficPerPathRule[prule].allowed() {
			err := ing.addEndpointsRuleV1(ic, ru.Host, prule, trafficPerPathRule[prule])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// addSpecIngressTLSV1 is used to add TLS Certificates from Ingress resources. Certificates will be added
// only if the Ingress rule host matches a host in TLS config
func (ing *ingress) addSpecIngressTLSV1(ic *ingressContext, ingtls *definitions.TLSV1) {
	ingressHosts := definitions.GetHostsFromIngressRulesV1(ic.ingressV1)

	// Hosts in the tls section need to explicitly match the host in the rules section.
	hostlist := compareStringList(ingtls.Hosts, ingressHosts)
	if len(hostlist) == 0 {
		ic.logger.Errorf("No matching tls hosts found - tls hosts: %s, ingress hosts: %s", ingtls.Hosts, ingressHosts)
		return
	} else if len(hostlist) != len(ingtls.Hosts) {
		ic.logger.Infof("Hosts in TLS and Ingress don't match: tls hosts: %s, ingress hosts: %s", ingtls.Hosts, definitions.GetHostsFromIngressRulesV1(ic.ingressV1))
	}

	// Skip adding certs to registry since no certs defined
	if ingtls.SecretName == "" {
		ic.logger.Debugf("No tls secret defined for hosts - %s", ingtls.Hosts)
		return
	}

	// Secrets should always reside in same namespace as the Ingress
	secretID := definitions.ResourceID{Name: ingtls.SecretName, Namespace: ic.ingressV1.Metadata.Namespace}
	secret, ok := ic.state.secrets[secretID]
	if !ok {
		ic.logger.Errorf("Failed to find secret %s in namespace %s", secretID.Name, secretID.Namespace)
		return
	}
	addTLSCertToRegistry(ic.certificateRegistry, ic.logger, hostlist, secret)
}

// converts the default backend if any
func (ing *ingress) convertDefaultBackendV1(
	ic *ingressContext,
	forceKubernetesService bool,
) (*eskip.Route, bool, error) {
	state := ic.state
	i := ic.ingressV1
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
		ic.logger.Errorf("Failed to get service %s, %s", svcName, svcPort)
		return nil, false, err
	}

	servicePort, err := svc.getServicePortV1(svcPort)
	if err != nil {
		ic.logger.Errorf("Failed to find target port %v, %s, for service %s add shuntroute: %v", svc.Spec.Ports, svcPort, svcName, err)
		err = nil
	} else if svc.Spec.Type == "ExternalName" {
		r, err := externalNameRoute(ns, name, "default", nil, svc, servicePort, ing.allowedExternalNames)
		return r, err == nil, err
	} else if forceKubernetesService {
		eps = []string{serviceNameBackend(svcName, ns, servicePort)}
	} else {
		ic.logger.Debugf("Found target port %v, for service %s", servicePort.TargetPort, svcName)
		protocol := "http"
		if p, ok := i.Metadata.Annotations[skipperBackendProtocolAnnotationKey]; ok {
			protocol = p
		}

		eps = state.GetEndpointsByService(
			ic.zone,
			ns,
			svcName,
			protocol,
			servicePort,
		)
		ic.logger.Debugf("Found %d endpoints for %s: %v", len(eps), svcName, err)
	}

	if len(eps) == 0 {
		ic.logger.Tracef("Target endpoints not found, shuntroute for %s:%s", svcName, svcPort)

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
		LBAlgorithm: getLoadBalancerAlgorithm(i.Metadata, ing.defaultLoadBalancerAlgorithm),
	}, true, nil
}

func serviceNameBackend(svcName, svcNamespace string, servicePort *servicePort) string {
	scheme := "https"
	if n, _ := servicePort.TargetPort.Number(); n != 443 {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.%s.svc.cluster.local:%s", scheme, svcName, svcNamespace, servicePort.TargetPort)
}

func (ing *ingress) ingressV1Route(
	i *definitions.IngressV1Item,
	redirect *redirectInfo,
	state *clusterState,
	hostRoutes map[string][]*eskip.Route,
	df defaultFilters,
	r *certregistry.CertRegistry,
	loggingEnabled bool,
) (*eskip.Route, error) {
	if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" || i.Spec == nil {
		log.Error("invalid ingress item: missing Metadata or Spec")
		return nil, nil
	}
	logger := newLogger("Ingress", i.Metadata.Namespace, i.Metadata.Name, loggingEnabled)

	redirect.initCurrent(i.Metadata)
	ic := &ingressContext{
		state:               state,
		ingressV1:           i,
		logger:              logger,
		annotationFilters:   annotationFilter(i.Metadata, logger),
		annotationPredicate: annotationPredicate(i.Metadata),
		annotationBackend:   annotationBackendString(i.Metadata),
		forwardBackendURL:   ing.forwardBackendURL,
		extraRoutes:         extraRoutes(i.Metadata),
		backendWeights:      backendWeights(i.Metadata, logger),
		pathMode:            pathMode(i.Metadata, ing.pathMode, logger),
		redirect:            redirect,
		hostRoutes:          hostRoutes,
		defaultFilters:      df,
		certificateRegistry: r,
		calculateTraffic:    getBackendTrafficCalculator[*weightedIngressBackend](ing.backendTrafficAlgorithm),
	}

	var route *eskip.Route
	if r, ok, err := ing.convertDefaultBackendV1(ic, ing.forceKubernetesService); ok {
		route = r
		ic.applyBackend(route)
	} else if err != nil {
		ic.logger.Errorf("Failed to convert default backend: %v", err)
	}

	for _, rule := range i.Spec.Rules {
		err := ing.addSpecRuleV1(ic, rule)
		if err != nil {
			return nil, err
		}
	}
	if ic.certificateRegistry != nil {
		for _, ingtls := range i.Spec.IngressTLS {
			ing.addSpecIngressTLSV1(ic, ingtls)
		}
	}
	return route, nil
}
