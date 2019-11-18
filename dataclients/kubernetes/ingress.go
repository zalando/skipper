package kubernetes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates/traffic"
)

const (
	defaultEastWestDomainRegexpPostfix = "[.]skipper[.]cluster[.]local"
	ingressRouteIDPrefix               = "kube"
	defaultLoadbalancerAlgorithm       = "roundRobin"
	backendWeightsAnnotationKey        = "zalando.org/backend-weights"
	ratelimitAnnotationKey             = "zalando.org/ratelimit"
	skipperfilterAnnotationKey         = "zalando.org/skipper-filter"
	skipperpredicateAnnotationKey      = "zalando.org/skipper-predicate"
	skipperRoutesAnnotationKey         = "zalando.org/skipper-routes"
	skipperLoadbalancerAnnotationKey   = "zalando.org/skipper-loadbalancer"
	pathModeAnnotationKey              = "zalando.org/skipper-ingress-path-mode"
	ingressOriginName                  = "ingress"
)

type ingressContext struct {
	state               *clusterState
	ingress             *ingressItem
	logger              *log.Entry
	annotationFilters   []*eskip.Filter
	annotationPredicate string
	extraRoutes         []*eskip.Route
	backendWeights      map[string]float64
	pathMode            PathMode
	redirect            *redirectInfo
	hostRoutes          map[string][]*eskip.Route
	defaultFilters      map[resourceId]string
}

type ingress struct {
	provideHTTPSRedirect        bool
	httpsRedirectCode           int
	pathMode                    PathMode
	kubernetesEnableEastWest    bool
	eastWestDomainRegexpPostfix string
}

var nonWord = regexp.MustCompile(`\W`)

func (ic *ingressContext) addHostRoute(host string, route *eskip.Route) {
	ic.hostRoutes[host] = append(ic.hostRoutes[host], route)
}

func newIngress(o Options, httpsRedirectCode int) *ingress {
	eastWestDomainRegexpPostfix := defaultEastWestDomainRegexpPostfix
	if o.KubernetesEastWestDomain != "" {
		if strings.HasPrefix(o.KubernetesEastWestDomain, ".") {
			o.KubernetesEastWestDomain = o.KubernetesEastWestDomain[1:len(o.KubernetesEastWestDomain)]
		}

		if strings.HasSuffix(o.KubernetesEastWestDomain, ".") {
			o.KubernetesEastWestDomain = o.KubernetesEastWestDomain[:len(o.KubernetesEastWestDomain)-1]
		}

		eastWestDomainRegexpPostfix = "[.]" + strings.Replace(o.KubernetesEastWestDomain, ".", "[.]", -1)
	}

	return &ingress{
		provideHTTPSRedirect:        o.ProvideHTTPSRedirect,
		httpsRedirectCode:           httpsRedirectCode,
		pathMode:                    o.PathMode,
		kubernetesEnableEastWest:    o.KubernetesEnableEastWest,
		eastWestDomainRegexpPostfix: eastWestDomainRegexpPostfix,
	}
}

func getServiceURL(svc *service, port backendPort) (string, error) {
	if p, ok := port.number(); ok {
		log.Debugf("service port as number: %d", p)
		return fmt.Sprintf("http://%s:%d", svc.Spec.ClusterIP, p), nil
	}

	pn, _ := port.name()
	for _, pi := range svc.Spec.Ports {
		if pi.Name == pn {
			log.Debugf("service port found by name: %s -> %d", pn, pi.Port)
			return fmt.Sprintf("http://%s:%d", svc.Spec.ClusterIP, pi.Port), nil
		}
	}

	log.Debugf("service port not found by name: %s", pn)
	return "", errServiceNotFound
}

func getLoadBalancerAlgorithm(m *metadata) string {
	algorithm := defaultLoadbalancerAlgorithm
	if algorithmAnnotationValue, ok := m.Annotations[skipperLoadbalancerAnnotationKey]; ok {
		algorithm = algorithmAnnotationValue
	}

	return algorithm
}

// TODO: find a nicer way to autogenerate route IDs
func routeID(namespace, name, host, path, backend string) string {
	namespace = nonWord.ReplaceAllString(namespace, "_")
	name = nonWord.ReplaceAllString(name, "_")
	host = nonWord.ReplaceAllString(host, "_")
	path = nonWord.ReplaceAllString(path, "_")
	backend = nonWord.ReplaceAllString(backend, "_")
	return fmt.Sprintf("%s_%s__%s__%s__%s__%s", ingressRouteIDPrefix, namespace, name, host, path, backend)
}

// routeIDForCustom generates a route id for a custom route of an ingress
// resource.
func routeIDForCustom(namespace, name, id, host string, index int) string {
	name = name + "_" + id + "_" + strconv.Itoa(index)
	return routeID(namespace, name, host, "", "")
}

func patchRouteID(rid string) string {
	return "kubeew" + rid[len(ingressRouteIDPrefix):]
}

func setPath(m PathMode, r *eskip.Route, p string) {
	if p == "" {
		return
	}

	switch m {
	case PathPrefix:
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: "PathSubtree",
			Args: []interface{}{p},
		})
	case PathRegexp:
		r.PathRegexps = []string{p}
	default:
		r.PathRegexps = []string{"^" + p}
	}
}

func convertPathRule(
	state *clusterState,
	metadata *metadata,
	host string,
	prule *pathRule,
	pathMode PathMode,
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
		hostRegexp = []string{"^" + strings.Replace(host, ".", "[.]", -1) + "$"}
	}
	svcPort := prule.Backend.ServicePort
	svcName := prule.Backend.ServiceName

	svc, err = state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertPathRule: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, err
	}

	targetPort, err := svc.getTargetPort(svcPort)
	if err != nil {
		// fallback to service, but service definition is wrong or no pods
		log.Errorf("Failed to find target port for service %s, fallback to service: %v", svcName, err)
		err = nil
	} else if svc.Spec.Type == "ExternalName" {
		scheme := "https"
		if targetPort != "443" {
			scheme = "http"
		}
		u := fmt.Sprintf("%s://%s:%s", scheme, svc.Spec.ExternalName, targetPort)
		f, e := eskip.ParseFilters(fmt.Sprintf(`setRequestHeader("Host", "%s")`, svc.Spec.ExternalName))
		if e != nil {
			return nil, e
		}
		return &eskip.Route{
			Id:          routeID(ns, name, "", "", svc.Spec.ExternalName),
			BackendType: eskip.NetworkBackend,
			Backend:     u,
			Filters:     f,
		}, nil
	} else {
		// err handled below
		eps, err = state.getEndpoints(ns, svcName, svcPort.String(), targetPort)
		log.Debugf("convertPathRule: Found %d endpoints %s for %s", len(eps), targetPort, svcName)
	}

	if len(eps) == 0 || err == errEndpointNotFound {

		address, err2 := getServiceURL(svc, svcPort)
		if err2 != nil {
			return nil, err2
		}
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     address,
			HostRegexps: hostRegexp,
		}

		setPath(pathMode, r, prule.Path)
		setTraffic(r, svcName, prule.Backend.Traffic, prule.Backend.noopCount)
		return r, nil

	} else if err != nil {
		return nil, err
	}
	log.Debugf("%d routes for %s/%s/%s", len(eps), ns, svcName, svcPort)

	if len(eps) == 1 {
		r := &eskip.Route{
			Id:          routeID(ns, name, host, prule.Path, svcName),
			Backend:     eps[0],
			HostRegexps: hostRegexp,
		}

		setPath(pathMode, r, prule.Path)
		setTraffic(r, svcName, prule.Backend.Traffic, prule.Backend.noopCount)
		return r, nil
	}

	if len(eps) == 0 {
		return nil, nil
	}

	r := &eskip.Route{
		Id:          routeID(ns, name, host, prule.Path, prule.Backend.ServiceName),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(metadata),
		HostRegexps: hostRegexp,
	}
	setPath(pathMode, r, prule.Path)
	setTraffic(r, svcName, prule.BackendTraffic, prule.Backend.noopCount)
	return r, nil
}

func setTraffic(r *eskip.Route, svcName string, weight float64, noopCount int) {
	// add traffic predicate if traffic weight is between 0.0 and 1.0
	if 0.0 < weight && weight < 1.0 {
		r.Predicates = append([]*eskip.Predicate{{
			Name: traffic.PredicateName,
			Args: []interface{}{weight},
		}}, r.Predicates...)
		log.Debugf("Traffic weight %.2f for backend '%s'", weight, svcName)
	}
	for i := 0; i < noopCount; i++ {
		r.Predicates = append([]*eskip.Predicate{{
			Name: primitive.NameTrue,
			Args: []interface{}{},
		}}, r.Predicates...)
	}
}

func applyAnnotationPredicates(m PathMode, r *eskip.Route, annotation string) error {
	if annotation == "" {
		return nil
	}

	predicates, err := eskip.ParsePredicates(annotation)
	if err != nil {
		return err
	}

	// to avoid conflict, give precedence for those predicates that come
	// from annotations
	if m == PathPrefix {
		for _, p := range predicates {
			if p.Name != "Path" && p.Name != "PathSubtree" {
				continue
			}

			r.Path = ""
			for i, p := range r.Predicates {
				if p.Name != "PathSubtree" && p.Name != "Path" {
					continue
				}

				copy(r.Predicates[i:], r.Predicates[i+1:])
				r.Predicates[len(r.Predicates)-1] = nil
				r.Predicates = r.Predicates[:len(r.Predicates)-1]
				break
			}
		}
	}

	r.Predicates = append(r.Predicates, predicates...)
	return nil
}

func defaultFiltersOf(service string, namespace string, defaultFilters map[resourceId]string) (string, bool) {
	if filters, ok := defaultFilters[resourceId{name: service, namespace: namespace}]; ok {
		return filters, true
	}
	return "", false
}

func (ing *ingress) addEndpointsRule(ic ingressContext, host string, prule *pathRule) error {
	endpointsRoute, err := convertPathRule(ic.state, ic.ingress.Metadata, host, prule, ic.pathMode)
	if err != nil {
		// if the service is not found the route should be removed
		if err == errServiceNotFound || err == errResourceNotFound {
			return nil
		}
		// Ingress status field does not support errors
		return fmt.Errorf("error while getting service: %v", err)
	}
	endpointsRoute.Filters = append(ic.annotationFilters, endpointsRoute.Filters...)
	// add pre-configured default filters
	if defFilter, ok := defaultFiltersOf(prule.Backend.ServiceName, ic.ingress.Metadata.Namespace, ic.defaultFilters); ok {
		defaultFilters, err := eskip.ParseFilters(defFilter)
		if err != nil {
			ic.logger.Errorf("Can not parse default filters: %v", err)
		} else {
			endpointsRoute.Filters = append(defaultFilters, endpointsRoute.Filters...)
		}
	}
	err = applyAnnotationPredicates(ic.pathMode, endpointsRoute, ic.annotationPredicate)
	if err != nil {
		ic.logger.Errorf("failed to apply annotation predicates: %v", err)
	}
	ic.addHostRoute(host, endpointsRoute)
	redirect := ic.redirect
	if redirect.enable || redirect.override {
		ic.addHostRoute(host, createIngressEnableHTTPSRedirect(endpointsRoute, redirect.code))
		redirect.setHost(host)
	}
	if redirect.disable {
		ic.addHostRoute(host, createIngressDisableHTTPSRedirect(endpointsRoute))
		redirect.setHostDisabled(host)
	}

	return nil
}

func addExtraRoutes(ic ingressContext, hosts []string, host string, path string) {
	// add extra routes from optional annotation
	for extraIndex, r := range ic.extraRoutes {
		route := *r
		route.HostRegexps = hosts
		route.Id = routeIDForCustom(
			ic.ingress.Metadata.Namespace,
			ic.ingress.Metadata.Name,
			route.Id,
			host+strings.Replace(path, "/", "_", -1),
			extraIndex)
		setPath(ic.pathMode, &route, path)
		if n := countPathRoutes(&route); n <= 1 {
			ic.addHostRoute(host, &route)
			ic.redirect.updateHost(host)
		} else {
			log.Errorf("Failed to add route having %d path routes: %v", n, r)
		}
	}
}

// computeBackendWeights computes and sets the backend traffic weights on the
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
func computeBackendWeights(backendWeights map[string]float64, rule *rule) {
	type pathInfo struct {
		sum        float64
		lastActive *backend
		count      int
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

		if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
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
			if weight, ok := backendWeights[path.Backend.ServiceName]; ok {
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
					path.Backend.noopCount = sc.weightsCount - 2
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

func (ing *ingress) addSpecRule(ic ingressContext, ru *rule) error {
	if ru.Http == nil {
		ic.logger.Warn("invalid ingress item: rule missing http definitions")
		return nil
	}
	// it is a regexp, would be better to have exact host, needs to be added in skipper
	// this wrapping is temporary and escaping is not the right thing to do
	// currently handled as mandatory
	host := []string{"^" + strings.Replace(ru.Host, ".", "[.]", -1) + "$"}
	// update Traffic field for each backend
	computeBackendWeights(ic.backendWeights, ru)
	for _, prule := range ru.Http.Paths {
		addExtraRoutes(ic, host, ru.Host, prule.Path)
		if prule.Backend.Traffic > 0 {
			err := ing.addEndpointsRule(ic, ru.Host, prule)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// converts the default backend if any
func (ing *ingress) convertDefaultBackend(state *clusterState, i *ingressItem) (*eskip.Route, bool, error) {
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
		svcName = i.Spec.DefaultBackend.ServiceName
		svcPort = i.Spec.DefaultBackend.ServicePort
	)

	svc, err := state.getService(ns, svcName)
	if err != nil {
		log.Errorf("convertDefaultBackend: Failed to get service %s, %s, %s", ns, svcName, svcPort)
		return nil, false, err
	}

	targetPort, err := svc.getTargetPort(svcPort)
	if err != nil {
		err = nil
		log.Errorf("Failed to find target port %v, %s, fallback to service", svc.Spec.Ports, svcPort)
	} else if svc.Spec.Type == "ExternalName" {
		scheme := "https"
		if targetPort != "443" {
			scheme = "http"
		}
		u := fmt.Sprintf("%s://%s:%s", scheme, svc.Spec.ExternalName, targetPort)
		f, err := eskip.ParseFilters(fmt.Sprintf(`setRequestHeader("Host", "%s")`, svc.Spec.ExternalName))
		if err != nil {
			return nil, false, err
		}
		return &eskip.Route{
			Id:          routeID(ns, name, "default", "", svc.Spec.ExternalName),
			BackendType: eskip.NetworkBackend,
			Backend:     u,
			Filters:     f,
		}, true, nil
	} else {
		// TODO(aryszka): check docs that service name is always good for requesting the endpoints
		log.Debugf("Found target port %v, for service %s", targetPort, svcName)
		eps, err = state.getEndpoints(
			ns,
			svcName,
			svcPort.String(),
			targetPort,
		)
		log.Debugf("convertDefaultBackend: Found %d endpoints for %s: %v", len(eps), svcName, err)
	}

	if len(eps) == 0 || err == errEndpointNotFound {

		address, err2 := getServiceURL(svc, svcPort)
		if err2 != nil {
			return nil, false, err2
		}

		return &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: address,
		}, true, nil
	} else if len(eps) == 1 {
		return &eskip.Route{
			Id:      routeID(ns, name, "", "", ""),
			Backend: eps[0],
		}, true, nil
	} else if err != nil {
		return nil, false, err
	}

	return &eskip.Route{
		Id:          routeID(ns, name, "", "", ""),
		BackendType: eskip.LBBackend,
		LBEndpoints: eps,
		LBAlgorithm: getLoadBalancerAlgorithm(i.Metadata),
	}, true, nil
}

func countPathRoutes(r *eskip.Route) int {
	i := 0
	for _, p := range r.Predicates {
		if p.Name == "PathSubtree" || p.Name == "Path" {
			i++
		}
	}
	if r.Path != "" {
		i++
	}
	return i
}

func createEastWestRoutes(eastWestDomainRegexpPostfix, name, ns string, routes []*eskip.Route) []*eskip.Route {
	ewroutes := make([]*eskip.Route, 0)
	newHostRegexps := []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ingressAlreadyHandled := false

	for _, r := range routes {
		// TODO(sszuecs) we have to rethink how to handle eastwest routes in more complex cases
		n := countPathRoutes(r)
		// FIX memory leak in route creation
		if strings.HasPrefix(r.Id, "kubeew") || (n == 0 && ingressAlreadyHandled) {
			continue
		}
		r.Namespace = ns // store namespace
		r.Name = name    // store name
		ewR := *r
		ewR.HostRegexps = newHostRegexps
		ewR.Id = patchRouteID(r.Id)
		ewroutes = append(ewroutes, &ewR)
		ingressAlreadyHandled = true
	}
	return ewroutes
}

func (ing *ingress) addEastWestRoutes(hostRoutes map[string][]*eskip.Route, i *ingressItem) {
	for _, rule := range i.Spec.Rules {
		if rs, ok := hostRoutes[rule.Host]; ok {
			rs = append(rs, createEastWestRoutes(ing.eastWestDomainRegexpPostfix, i.Metadata.Name, i.Metadata.Namespace, rs)...)
			hostRoutes[rule.Host] = rs
		}
	}
}

// parse filter and ratelimit annotation
func annotationFilter(i *ingressItem, logger *log.Entry) []*eskip.Filter {
	var annotationFilter string
	if ratelimitAnnotationValue, ok := i.Metadata.Annotations[ratelimitAnnotationKey]; ok {
		annotationFilter = ratelimitAnnotationValue
	}
	if val, ok := i.Metadata.Annotations[skipperfilterAnnotationKey]; ok {
		if annotationFilter != "" {
			annotationFilter += " -> "
		}
		annotationFilter += val
	}

	if annotationFilter != "" {
		annotationFilters, err := eskip.ParseFilters(annotationFilter)
		if err == nil {
			return annotationFilters
		}
		logger.Errorf("Can not parse annotation filters: %v", err)
	}
	return nil
}

// parse predicate annotation
func annotationPredicate(i *ingressItem) string {
	var annotationPredicate string
	if val, ok := i.Metadata.Annotations[skipperpredicateAnnotationKey]; ok {
		annotationPredicate = val
	}
	return annotationPredicate
}

// parse routes annotation
func extraRoutes(i *ingressItem, logger *log.Entry) []*eskip.Route {
	var extraRoutes []*eskip.Route
	annotationRoutes := i.Metadata.Annotations[skipperRoutesAnnotationKey]
	if annotationRoutes != "" {
		var err error
		extraRoutes, err = eskip.Parse(annotationRoutes)
		if err != nil {
			logger.Errorf("failed to parse routes from %s, skipping: %v", skipperRoutesAnnotationKey, err)
		}
	}
	return extraRoutes
}

// parse backend-weights annotation if it exists
func backendWeights(i *ingressItem, logger *log.Entry) map[string]float64 {
	var backendWeights map[string]float64
	if backends, ok := i.Metadata.Annotations[backendWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(backends), &backendWeights)
		if err != nil {
			logger.Errorf("error while parsing backend-weights annotation: %v", err)
		}
	}
	return backendWeights
}

// parse pathmode from annotation or fallback to global default
func pathMode(i *ingressItem, globalDefault PathMode) PathMode {
	pathMode := globalDefault

	if pathModeString, ok := i.Metadata.Annotations[pathModeAnnotationKey]; ok {
		if p, err := ParsePathMode(pathModeString); err != nil {
			log.Errorf("Failed to get path mode for ingress %s/%s: %v", i.Metadata.Namespace, i.Metadata.Name, err)
		} else {
			log.Debugf("Set pathMode to %s", p)
			pathMode = p
		}
	}
	return pathMode
}

func (ing *ingress) ingressRoute(i *ingressItem, redirect *redirectInfo, state *clusterState,
	hostRoutes map[string][]*eskip.Route, defaultFilters map[resourceId]string) (*eskip.Route, error) {
	if i.Metadata == nil || i.Metadata.Namespace == "" || i.Metadata.Name == "" ||
		i.Spec == nil {
		log.Error("invalid ingress item: missing metadata")
		return nil, nil
	}
	logger := log.WithFields(log.Fields{
		"ingress": fmt.Sprintf("%s/%s", i.Metadata.Namespace, i.Metadata.Name),
	})
	redirect.initCurrent(i.Metadata)
	ic := ingressContext{
		state:               state,
		ingress:             i,
		logger:              logger,
		annotationFilters:   annotationFilter(i, logger),
		annotationPredicate: annotationPredicate(i),
		extraRoutes:         extraRoutes(i, logger),
		backendWeights:      backendWeights(i, logger),
		pathMode:            pathMode(i, ing.pathMode),
		redirect:            redirect,
		hostRoutes:          hostRoutes,
		defaultFilters:      defaultFilters,
	}

	var route *eskip.Route
	if r, ok, err := ing.convertDefaultBackend(state, i); ok {
		route = r
	} else if err != nil {
		ic.logger.Errorf("error while converting default backend: %v", err)
	}
	for _, rule := range i.Spec.Rules {
		err := ing.addSpecRule(ic, rule)
		if err != nil {
			return nil, err
		}
	}
	if ing.kubernetesEnableEastWest {
		ing.addEastWestRoutes(hostRoutes, i)
	}
	return route, nil
}

func createEastWestRoute(eastWestDomainRegexpPostfix, name, ns string, r *eskip.Route) *eskip.Route {
	if strings.HasPrefix(r.Id, "kubeew") || ns == "" || name == "" {
		return nil
	}
	ewR := *r
	ewR.HostRegexps = []string{"^" + name + "[.]" + ns + eastWestDomainRegexpPostfix + "$"}
	ewR.Id = patchRouteID(r.Id)
	return &ewR
}

func (ing *ingress) addCatchAllRoutes(host string, r *eskip.Route, redirect *redirectInfo) []*eskip.Route {
	catchAll := &eskip.Route{
		Id:          routeID("", "catchall", host, "", ""),
		HostRegexps: r.HostRegexps,
		BackendType: eskip.ShuntBackend,
	}
	routes := []*eskip.Route{catchAll}
	if ing.kubernetesEnableEastWest {
		if ew := createEastWestRoute(ing.eastWestDomainRegexpPostfix, r.Name, r.Namespace, catchAll); ew != nil {
			routes = append(routes, ew)
		}
	}
	if code, ok := redirect.setHostCode[host]; ok {
		routes = append(routes, createIngressEnableHTTPSRedirect(catchAll, code))
	}
	if redirect.disableHost[host] {
		routes = append(routes, createIngressDisableHTTPSRedirect(catchAll))
	}

	return routes
}

// catchAllRoutes returns true if one of the routes in the list has a catchAll
// path expression.
func catchAllRoutes(routes []*eskip.Route) bool {
	for _, route := range routes {
		if len(route.PathRegexps) == 0 {
			return true
		}

		for _, exp := range route.PathRegexps {
			if exp == "^/" {
				return true
			}
		}
	}

	return false
}

// ingressToRoutes logs if an invalid found, but proceeds with the
// valid ones.  Reporting failures in Ingress status is not possible,
// because Ingress status field is v1.LoadBalancerIngress that only
// supports IP and Hostname as string.
func (ing *ingress) ingressToRoutes(state *clusterState, defaultFilters map[resourceId]string) ([]*eskip.Route, error) {
	routes := make([]*eskip.Route, 0, len(state.ingresses))
	hostRoutes := make(map[string][]*eskip.Route)
	redirect := createRedirectInfo(ing.provideHTTPSRedirect, ing.httpsRedirectCode)
	for _, i := range state.ingresses {
		r, err := ing.ingressRoute(i, redirect, state, hostRoutes, defaultFilters)
		if err != nil {
			return nil, err
		}
		if r != nil {
			routes = append(routes, r)
		}
	}

	for host, rs := range hostRoutes {
		if len(rs) == 0 {
			continue
		}

		routes = append(routes, rs...)

		// if routes were configured, but there is no catchall route
		// defined for the host name, create a route which returns 404
		if !catchAllRoutes(rs) {
			routes = append(routes, ing.addCatchAllRoutes(host, rs[0], redirect)...)
		}
	}

	return routes, nil
}

func (ing *ingress) convert(s *clusterState, defaultFilters map[resourceId]string) ([]*eskip.Route, error) {
	r, err := ing.ingressToRoutes(s, defaultFilters)
	if err != nil {
		return nil, err
	}

	return r, nil
}
