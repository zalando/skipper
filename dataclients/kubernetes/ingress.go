package kubernetes

import (
	"crypto/tls"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	extraRoutes         []*eskip.Route
	backendWeights      map[string]float64
	pathMode            PathMode
	redirect            *redirectInfo
	hostRoutes          map[string][]*eskip.Route
	defaultFilters      defaultFilters
	certificateRegistry *certregistry.CertRegistry
	calculateTraffic    func([]*weightedIngressBackend) map[string]backendTraffic
}

type ingress struct {
	eastWestRangeDomains         []string
	eastWestRangePredicates      []*eskip.Predicate
	allowedExternalNames         []*regexp.Regexp
	kubernetesEastWestDomain     string
	pathMode                     PathMode
	httpsRedirectCode            int
	kubernetesEnableEastWest     bool
	provideHTTPSRedirect         bool
	forceKubernetesService       bool
	backendTrafficAlgorithm      BackendTrafficAlgorithm
	defaultLoadBalancerAlgorithm string
}

var nonWord = regexp.MustCompile(`\W`)

var errNotAllowedExternalName = errors.New("ingress with not allowed external name service")

func (ic *ingressContext) addHostRoute(host string, route *eskip.Route) {
	ic.hostRoutes[host] = append(ic.hostRoutes[host], route)
}

func newIngress(o Options) *ingress {
	return &ingress{
		provideHTTPSRedirect:         o.ProvideHTTPSRedirect,
		httpsRedirectCode:            o.HTTPSRedirectCode,
		pathMode:                     o.PathMode,
		kubernetesEnableEastWest:     o.KubernetesEnableEastWest,
		kubernetesEastWestDomain:     o.KubernetesEastWestDomain,
		eastWestRangeDomains:         o.KubernetesEastWestRangeDomains,
		eastWestRangePredicates:      o.KubernetesEastWestRangePredicates,
		allowedExternalNames:         o.AllowedExternalNames,
		forceKubernetesService:       o.ForceKubernetesService,
		backendTrafficAlgorithm:      o.BackendTrafficAlgorithm,
		defaultLoadBalancerAlgorithm: o.DefaultLoadBalancerAlgorithm,
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

func addExtraRoutes(ic *ingressContext, ruleHost, path, pathType, eastWestDomain string, enableEastWest bool) {
	hosts := []string{createHostRx(ruleHost)}
	var ns, name string
	name = ic.ingressV1.Metadata.Name
	ns = ic.ingressV1.Metadata.Namespace

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
		if n := countPathRoutes(&route); n <= 1 {
			ic.addHostRoute(ruleHost, &route)
			ic.redirect.updateHost(ruleHost)
		} else {
			ic.logger.Errorf("Failed to add route having %d path routes: %v", n, r)
		}
		if enableEastWest {
			ewRoute := createEastWestRouteIng(eastWestDomain, name, ns, &route)
			ewHost := fmt.Sprintf("%s.%s.%s", name, ns, eastWestDomain)
			ic.addHostRoute(ewHost, ewRoute)
		}
	}
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
		logger.Errorf("Can not parse annotation filters: %v", err)
	}
	return nil
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
func extraRoutes(m *definitions.Metadata, logger *logger) []*eskip.Route {
	var extraRoutes []*eskip.Route
	annotationRoutes := m.Annotations[definitions.IngressRoutesAnnotation]
	if annotationRoutes != "" {
		var err error
		extraRoutes, err = eskip.Parse(annotationRoutes)
		if err != nil {
			logger.Errorf("Failed to parse routes from %s, skipping: %v", definitions.IngressRoutesAnnotation, err)
		}
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

// parse pathmode from annotation or fallback to global default
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

// addHostTLSCert adds a TLS certificate to the certificate registry per host when the referenced
// secret is found and is a valid TLS secret.
func addHostTLSCert(ic *ingressContext, hosts []string, secretID *definitions.ResourceID) {
	secret, ok := ic.state.secrets[*secretID]
	if !ok {
		ic.logger.Errorf("Failed to find secret %s in namespace %s", secretID.Name, secretID.Namespace)
		return
	}
	cert, err := generateTLSCertFromSecret(secret)
	if err != nil {
		ic.logger.Errorf("Failed to generate TLS certificate from secret: %v", err)
		return
	}
	for _, host := range hosts {
		err := ic.certificateRegistry.ConfigureCertificate(host, cert)
		if err != nil {
			ic.logger.Errorf("Failed to configure certificate: %v", err)
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
	}

	for host, rs := range hostRoutes {
		if len(rs) == 0 {
			continue
		}

		applyEastWestRange(ing.eastWestRangeDomains, ing.eastWestRangePredicates, host, rs)
		routes = append(routes, rs...)
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
