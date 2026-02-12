package kubernetes

import (
	"crypto/tls"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/secrets/certregistry"
)

const (
	ingressRouteIDPrefix                = "kube"
	backendWeightsAnnotationKey         = "zalando.org/backend-weights"
	ratelimitAnnotationKey              = "zalando.org/ratelimit"
	skipperLoadBalancerAnnotationKey    = "zalando.org/skipper-loadbalancer"
	skipperBackendProtocolAnnotationKey = "zalando.org/skipper-backend-protocol"
	pathModeAnnotationKey               = "zalando.org/skipper-ingress-path-mode"
	ingressOriginName                   = "ingress"
	tlsSecretType                       = "kubernetes.io/tls"
	tlsSecretDataCrt                    = "tls.crt"
	tlsSecretDataKey                    = "tls.key"
)

type ingressContext struct {
	state               *clusterState
	ingressV1           *definitions.IngressV1Item
	logger              *logger
	annotationFilters   []*eskip.Filter
	annotationPredicate string
	annotationBackend   string
	forwardBackendURL   string
	enableExternalNames bool
	extraRoutes         []*eskip.Route
	backendWeights      map[string]float64
	pathMode            PathMode
	redirect            *redirectInfo
	hostRoutes          map[string][]*eskip.Route
	defaultFilters      defaultFilters
	certificateRegistry *certregistry.CertRegistry
	calculateTraffic    func([]*weightedIngressBackend) map[string]backendTraffic
	zone                string
}

type ingress struct {
	eastWestRangeDomains                           []string
	eastWestRangePredicates                        []*eskip.Predicate
	allowedExternalNames                           []*regexp.Regexp
	kubernetesEastWestDomain                       string
	zone                                           string
	pathMode                                       PathMode
	httpsRedirectCode                              int
	kubernetesEnableEastWest                       bool
	provideHTTPSRedirect                           bool
	disableCatchAllRoutes                          bool
	forceKubernetesService                         bool
	enableExternalNames                            bool
	backendTrafficAlgorithm                        BackendTrafficAlgorithm
	defaultLoadBalancerAlgorithm                   string
	forwardBackendURL                              string
	kubernetesAnnotationPredicates                 []AnnotationPredicates
	kubernetesAnnotationFiltersAppend              []AnnotationFilters
	kubernetesEastWestRangeAnnotationPredicates    []AnnotationPredicates
	kubernetesEastWestRangeAnnotationFiltersAppend []AnnotationFilters
}

var (
	nonWord = regexp.MustCompile(`\W`)

	errNotEnabledExternalName = errors.New("ingress is not enabled to reference external name service")
	errNotAllowedExternalName = errors.New("ingress with not allowed external name service")
)

func (ic *ingressContext) addHostRoute(host string, route *eskip.Route) {
	ic.applyBackend(route)
	ic.hostRoutes[host] = append(ic.hostRoutes[host], route)
}

func (ic *ingressContext) applyBackend(route *eskip.Route) {
	if ic.forwardBackendURL == "" || ic.annotationBackend == "" || route == nil {
		return
	}
	if be, err := eskip.BackendTypeFromString(ic.annotationBackend); err != nil {
		return
	} else {
		switch be {
		case eskip.ForwardBackend:
			route.BackendType = eskip.NetworkBackend
			route.Backend = ic.forwardBackendURL
			route.Filters = []*eskip.Filter{}
		}
	}
}

func newIngress(o Options) *ingress {
	return &ingress{
		provideHTTPSRedirect:                           o.ProvideHTTPSRedirect,
		httpsRedirectCode:                              o.HTTPSRedirectCode,
		disableCatchAllRoutes:                          o.DisableCatchAllRoutes,
		pathMode:                                       o.PathMode,
		kubernetesEnableEastWest:                       o.KubernetesEnableEastWest,
		kubernetesEastWestDomain:                       o.KubernetesEastWestDomain,
		eastWestRangeDomains:                           o.KubernetesEastWestRangeDomains,
		eastWestRangePredicates:                        o.KubernetesEastWestRangePredicates,
		enableExternalNames:                            o.EnableExternalNames,
		zone:                                           o.TopologyZone,
		allowedExternalNames:                           o.AllowedExternalNames,
		forceKubernetesService:                         o.ForceKubernetesService,
		backendTrafficAlgorithm:                        o.BackendTrafficAlgorithm,
		defaultLoadBalancerAlgorithm:                   o.DefaultLoadBalancerAlgorithm,
		forwardBackendURL:                              o.ForwardBackendURL,
		kubernetesAnnotationPredicates:                 o.KubernetesAnnotationPredicates,
		kubernetesAnnotationFiltersAppend:              o.KubernetesAnnotationFiltersAppend,
		kubernetesEastWestRangeAnnotationPredicates:    o.KubernetesEastWestRangeAnnotationPredicates,
		kubernetesEastWestRangeAnnotationFiltersAppend: o.KubernetesEastWestRangeAnnotationFiltersAppend,
	}
}

func getLoadBalancerAlgorithm(m *definitions.Metadata, defaultAlgorithm string) string {
	algorithm := defaultAlgorithm
	if algorithmAnnotationValue, ok := m.Annotations[skipperLoadBalancerAnnotationKey]; ok {
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

func externalNameRoute(
	ns, name, idHost string,
	hostRegexps []string,
	svc *service,
	servicePort *servicePort,
	allowedNames []*regexp.Regexp,
) (*eskip.Route, error) {
	if !isExternalDomainAllowed(allowedNames, svc.Spec.ExternalName) {
		return nil, fmt.Errorf("%w: %s", errNotAllowedExternalName, svc.Spec.ExternalName)
	}

	scheme := "https"
	if n, _ := servicePort.TargetPort.Number(); n != 443 {
		scheme = "http"
	}

	u := fmt.Sprintf("%s://%s:%s", scheme, svc.Spec.ExternalName, servicePort.TargetPort)
	f, err := eskip.ParseFilters(fmt.Sprintf(`setRequestHeader("Host", "%s")`, svc.Spec.ExternalName))
	if err != nil {
		return nil, err
	}

	return &eskip.Route{
		Id:          routeID(ns, name, idHost, "", svc.Spec.ExternalName),
		BackendType: eskip.NetworkBackend,
		Backend:     u,
		Filters:     f,
		HostRegexps: hostRegexps,
	}, nil
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

func (ing *ingress) addExtraRoutes(ic *ingressContext, ruleHost, path, pathType string) {
	hosts := []string{createHostRx(ruleHost)}
	var ns, name string
	name = ic.ingressV1.Metadata.Name
	ns = ic.ingressV1.Metadata.Namespace
	eastWestDomain := ing.kubernetesEastWestDomain
	enableEastWest := ing.kubernetesEnableEastWest
	ewHost := isEastWestHost(ruleHost, ing.eastWestRangeDomains)
	// add extra routes from optional annotation
	for extraIndex, r := range ic.extraRoutes {
		route := *r
		route.HostRegexps = hosts
		route.Id = routeIDForCustom(
			ns,
			name,
			route.Id,
			ruleHost+strings.ReplaceAll(path, "/", "_"),
			extraIndex)
		setPathV1(ic.pathMode, &route, pathType, path)
		if n := countPathPredicates(&route); n <= 1 {
			if ewHost {
				appendAnnotationPredicates(ing.kubernetesEastWestRangeAnnotationPredicates, ic.ingressV1.Metadata.Annotations, &route)
				appendAnnotationFilters(ing.kubernetesEastWestRangeAnnotationFiltersAppend, ic.ingressV1.Metadata.Annotations, &route)
			} else {
				appendAnnotationPredicates(ing.kubernetesAnnotationPredicates, ic.ingressV1.Metadata.Annotations, &route)
				appendAnnotationFilters(ing.kubernetesAnnotationFiltersAppend, ic.ingressV1.Metadata.Annotations, &route)
			}
			ic.addHostRoute(ruleHost, &route)
			ic.redirect.updateHost(ruleHost)
		} else {
			ic.logger.Errorf("Ignoring route due to multiple path predicates: %d path predicates, route: %v", n, route)
		}
		if enableEastWest {
			ewRoute := createEastWestRouteIng(eastWestDomain, name, ns, &route)
			ewHost := fmt.Sprintf("%s.%s.%s", name, ns, eastWestDomain)
			ic.addHostRoute(ewHost, ewRoute)
		}
	}
}

func countPathPredicates(r *eskip.Route) int {
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

// parse filter and ratelimit annotation
func annotationFilter(m *definitions.Metadata, logger *logger) []*eskip.Filter {
	var annotationFilter string
	if ratelimitAnnotationValue, ok := m.Annotations[ratelimitAnnotationKey]; ok {
		annotationFilter = ratelimitAnnotationValue
	}
	if val, ok := m.Annotations[definitions.IngressFilterAnnotation]; ok {
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
		logger.Errorf("Cannot parse annotation filters: %v", err)
	}
	return nil
}

// parse backend annotation
func annotationBackend(m *definitions.Metadata) (eskip.BackendType, error) {
	if s, ok := m.Annotations[definitions.IngressBackendAnnotation]; ok {
		return eskip.BackendTypeFromString(s)
	}
	return 0, fmt.Errorf("annotation not found")
}

func annotationBackendString(m *definitions.Metadata) string {
	if be, err := annotationBackend(m); err == nil {
		return be.String()
	}
	return ""
}

// parse predicate annotation
func annotationPredicate(m *definitions.Metadata) string {
	var annotationPredicate string
	if val, ok := m.Annotations[definitions.IngressPredicateAnnotation]; ok {
		annotationPredicate = val
	}
	return annotationPredicate
}

// parse routes annotation
func extraRoutes(m *definitions.Metadata) []*eskip.Route {
	var extraRoutes []*eskip.Route
	annotationRoutes := m.Annotations[definitions.IngressRoutesAnnotation]
	if annotationRoutes != "" {
		extraRoutes, _ = eskip.Parse(annotationRoutes) // We ignore the error here because it should be handled by the validator object
	}
	return extraRoutes
}

// parse backend-weights annotation if it exists
func backendWeights(m *definitions.Metadata, logger *logger) map[string]float64 {
	var backendWeights map[string]float64
	if backends, ok := m.Annotations[backendWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(backends), &backendWeights)
		if err != nil {
			logger.Errorf("Error while parsing backend-weights annotation: %v", err)
		}
	}
	return backendWeights
}

// parse pathmode from annotation or fall back to global default
func pathMode(m *definitions.Metadata, globalDefault PathMode, logger *logger) PathMode {
	pathMode := globalDefault

	if pathModeString, ok := m.Annotations[pathModeAnnotationKey]; ok {
		if p, err := ParsePathMode(pathModeString); err != nil {
			logger.Errorf("Failed to get path mode: %v", err)
		} else {
			logger.Debugf("Set pathMode to %s", p)
			pathMode = p
		}
	}
	return pathMode
}

func (ing *ingress) addCatchAllRoutes(host string, r *eskip.Route, redirect *redirectInfo) []*eskip.Route {
	catchAll := &eskip.Route{
		Id:          routeID("", "catchall", host, "", ""),
		HostRegexps: r.HostRegexps,
		BackendType: eskip.ShuntBackend,
	}
	routes := []*eskip.Route{catchAll}
	if ing.kubernetesEnableEastWest {
		if ew := createEastWestRouteIng(ing.kubernetesEastWestDomain, r.Name, r.Namespace, catchAll); ew != nil {
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

// hasCatchAllRoutes returns true if one of the routes in the list has a catchAll
// path expression.
//
// TODO: this should also consider path types exact and subtree
func hasCatchAllRoutes(routes []*eskip.Route) bool {
	for _, route := range routes {
		if len(route.PathRegexps) == 0 {
			return true
		}

		if slices.Contains(route.PathRegexps, "^/") {
			return true
		}
	}

	return false
}

func (ing *ingress) applyBackend(i *definitions.IngressV1Item, r *eskip.Route) {
	if ing.forwardBackendURL == "" || r == nil {
		return
	}
	if be, err := annotationBackend(i.Metadata); err != nil {
		return
	} else {
		switch be {
		case eskip.ForwardBackend:
			r.BackendType = eskip.NetworkBackend
			r.Backend = ing.forwardBackendURL
			r.Filters = []*eskip.Filter{}
		}
	}
}

// convert logs if an invalid found, but proceeds with the valid ones.
// Reporting failures in Ingress status is not possible, because
// Ingress status field only supports IP and Hostname as string.
func (ing *ingress) convert(state *clusterState, df defaultFilters, r *certregistry.CertRegistry, loggingEnabled bool) ([]*eskip.Route, error) {
	var ewIngInfo map[string][]string // r.Id -> {namespace, name}
	if ing.kubernetesEnableEastWest {
		ewIngInfo = make(map[string][]string)
	}
	routes := make([]*eskip.Route, 0, len(state.ingressesV1))
	hostRoutes := make(map[string][]*eskip.Route)
	redirect := createRedirectInfo(ing.provideHTTPSRedirect, ing.httpsRedirectCode)
	for _, i := range state.ingressesV1 {
		r, err := ing.ingressV1Route(i, redirect, state, hostRoutes, df, r, loggingEnabled)
		if err != nil {
			return nil, err
		}
		if r != nil {
			routes = append(routes, r)
			if ing.kubernetesEnableEastWest {
				ewIngInfo[r.Id] = []string{i.Metadata.Namespace, i.Metadata.Name}
			}
		}
		ing.applyBackend(i, r)
	}

	for host, rs := range hostRoutes {
		if len(rs) == 0 {
			continue
		}

		applyEastWestRange(ing.eastWestRangeDomains, ing.eastWestRangePredicates, host, rs)
		routes = append(routes, rs...)

		if !ing.disableCatchAllRoutes {
			// if routes were configured, but there is no catchall route
			// defined for the host name, create a route which returns 404
			if !hasCatchAllRoutes(rs) {
				routes = append(routes, ing.addCatchAllRoutes(host, rs[0], redirect)...)
			}
		}
	}

	if ing.kubernetesEnableEastWest && len(routes) > 0 && len(ewIngInfo) > 0 {
		ewroutes := make([]*eskip.Route, 0, len(routes))
		for _, r := range routes {
			if v, ok := ewIngInfo[r.Id]; ok {
				ewroutes = append(ewroutes, createEastWestRouteIng(ing.kubernetesEastWestDomain, v[0], v[1], r))
			}
		}
		l := len(routes)
		routes = append(routes, ewroutes...)
		log.Infof("Enabled east west routes: %d %d %d %d", l, len(routes), len(ewroutes), len(hostRoutes))
	}

	return routes, nil
}

func generateTLSCertFromSecret(secret *secret) (*tls.Certificate, error) {
	if secret.Data[tlsSecretDataCrt] == "" || secret.Data[tlsSecretDataKey] == "" {
		return nil, fmt.Errorf("secret must contain %s and %s in data field", tlsSecretDataCrt, tlsSecretDataKey)
	}
	crt, err := b64.StdEncoding.DecodeString(secret.Data[tlsSecretDataCrt])
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s from secret %s", tlsSecretDataCrt, secret.Metadata.Name)
	}
	key, err := b64.StdEncoding.DecodeString(secret.Data[tlsSecretDataKey])
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s from secret %s", tlsSecretDataKey, secret.Metadata.Name)
	}
	cert, err := tls.X509KeyPair([]byte(crt), []byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to create tls certificate from secret %s", secret.Metadata.Name)
	}
	if secret.Type != tlsSecretType {
		return nil, fmt.Errorf("secret %s is not of type %s", secret.Metadata.Name, tlsSecretType)
	}
	return &cert, nil
}
