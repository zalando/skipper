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
	"github.com/zalando/skipper/certregistry"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

const (
	ingressRouteIDPrefix                = "kube"
	backendWeightsAnnotationKey         = "zalando.org/backend-weights"
	ratelimitAnnotationKey              = "zalando.org/ratelimit"
	skipperfilterAnnotationKey          = "zalando.org/skipper-filter"
	skipperpredicateAnnotationKey       = "zalando.org/skipper-predicate"
	skipperRoutesAnnotationKey          = "zalando.org/skipper-routes"
	skipperLoadBalancerAnnotationKey    = "zalando.org/skipper-loadbalancer"
	skipperBackendProtocolAnnotationKey = "zalando.org/skipper-backend-protocol"
	pathModeAnnotationKey               = "zalando.org/skipper-ingress-path-mode"
	ingressOriginName                   = "ingress"
	tlsSecretType                       = "kubernetes.io/tls"
)

type ingressContext struct {
	state               *clusterState
	ingress             *definitions.IngressItem
	ingressV1           *definitions.IngressV1Item
	logger              *log.Entry
	annotationFilters   []*eskip.Filter
	annotationPredicate string
	extraRoutes         []*eskip.Route
	backendWeights      map[string]float64
	pathMode            PathMode
	redirect            *redirectInfo
	hostRoutes          map[string][]*eskip.Route
	defaultFilters      defaultFilters
	certificateRegistry *certregistry.CertRegistry
}

type ingress struct {
	eastWestRangeDomains     []string
	eastWestRangePredicates  []*eskip.Predicate
	allowedExternalNames     []*regexp.Regexp
	kubernetesEastWestDomain string
	pathMode                 PathMode
	httpsRedirectCode        int
	kubernetesEnableEastWest bool
	ingressV1                bool
	provideHTTPSRedirect     bool
}

var nonWord = regexp.MustCompile(`\W`)

var errNotAllowedExternalName = errors.New("ingress with not allowed external name service")

func (ic *ingressContext) addHostRoute(host string, route *eskip.Route) {
	ic.hostRoutes[host] = append(ic.hostRoutes[host], route)
}

func newIngress(o Options) *ingress {
	return &ingress{
		ingressV1:                o.KubernetesIngressV1,
		provideHTTPSRedirect:     o.ProvideHTTPSRedirect,
		httpsRedirectCode:        o.HTTPSRedirectCode,
		pathMode:                 o.PathMode,
		kubernetesEnableEastWest: o.KubernetesEnableEastWest,
		kubernetesEastWestDomain: o.KubernetesEastWestDomain,
		eastWestRangeDomains:     o.KubernetesEastWestRangeDomains,
		eastWestRangePredicates:  o.KubernetesEastWestRangePredicates,
		allowedExternalNames:     o.AllowedExternalNames,
	}
}

func getLoadBalancerAlgorithm(m *definitions.Metadata) string {
	algorithm := defaultLoadBalancerAlgorithm
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

func setTraffic(r *eskip.Route, svcName string, weight float64, noopCount int) {
	// add traffic predicate if traffic weight is between 0.0 and 1.0
	if 0.0 < weight && weight < 1.0 {
		r.Predicates = append([]*eskip.Predicate{{
			Name: predicates.TrafficName,
			Args: []interface{}{weight},
		}}, r.Predicates...)
		log.Debugf("Traffic weight %.2f for backend '%s'", weight, svcName)
	}
	for i := 0; i < noopCount; i++ {
		r.Predicates = append([]*eskip.Predicate{{
			Name: predicates.TrueName,
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

func addExtraRoutes(ic ingressContext, ruleHost, path, pathType, eastWestDomain string, enableEastWest bool) {
	hosts := []string{createHostRx(ruleHost)}
	var ns, name string
	if ic.ingressV1 != nil {
		name = ic.ingressV1.Metadata.Name
		ns = ic.ingressV1.Metadata.Namespace

	} else {
		name = ic.ingress.Metadata.Name
		ns = ic.ingress.Metadata.Namespace
	}

	// add extra routes from optional annotation
	for extraIndex, r := range ic.extraRoutes {
		route := *r
		route.HostRegexps = hosts
		route.Id = routeIDForCustom(
			ns,
			name,
			route.Id,
			ruleHost+strings.Replace(path, "/", "_", -1),
			extraIndex)
		setPathV1(ic.pathMode, &route, pathType, path)
		if n := countPathRoutes(&route); n <= 1 {
			ic.addHostRoute(ruleHost, &route)
			ic.redirect.updateHost(ruleHost)
		} else {
			log.Errorf("Failed to add route having %d path routes: %v", n, r)
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
func annotationFilter(m *definitions.Metadata, logger *log.Entry) []*eskip.Filter {
	var annotationFilter string
	if ratelimitAnnotationValue, ok := m.Annotations[ratelimitAnnotationKey]; ok {
		annotationFilter = ratelimitAnnotationValue
	}
	if val, ok := m.Annotations[skipperfilterAnnotationKey]; ok {
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
	if val, ok := m.Annotations[skipperpredicateAnnotationKey]; ok {
		annotationPredicate = val
	}
	return annotationPredicate
}

// parse routes annotation
func extraRoutes(m *definitions.Metadata, logger *log.Entry) []*eskip.Route {
	var extraRoutes []*eskip.Route
	annotationRoutes := m.Annotations[skipperRoutesAnnotationKey]
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
func backendWeights(m *definitions.Metadata, logger *log.Entry) map[string]float64 {
	var backendWeights map[string]float64
	if backends, ok := m.Annotations[backendWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(backends), &backendWeights)
		if err != nil {
			logger.Errorf("error while parsing backend-weights annotation: %v", err)
		}
	}
	return backendWeights
}

// parse pathmode from annotation or fallback to global default
func pathMode(m *definitions.Metadata, globalDefault PathMode) PathMode {
	pathMode := globalDefault

	if pathModeString, ok := m.Annotations[pathModeAnnotationKey]; ok {
		if p, err := ParsePathMode(pathModeString); err != nil {
			log.Errorf("Failed to get path mode for ingress %s/%s: %v", m.Namespace, m.Name, err)
		} else {
			log.Debugf("Set pathMode to %s", p)
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

		for _, exp := range route.PathRegexps {
			if exp == "^/" {
				return true
			}
		}
	}

	return false
}

func addHostTLSCerts(ic ingressContext, hosts []string, secretName string, ns string) error {
	var (
		err   error
		found bool
	)

	for _, secret := range ic.state.secrets {
		if (secret.Meta.Name == secretName) && (secret.Meta.Namespace == ns) {
			found = true
			if secret.Type != tlsSecretType {
				log.Warnf("ingress tls secret %s is not of type %s", secretName, tlsSecretType)
			}
			if secret.Data["tls.crt"] == "" || secret.Data["tls.key"] == "" {
				log.Errorf("failed to use %s for TLS, secret must contain tls.crt and tls.key in data field", secretName)
				return err
			}
			crt, err := b64.StdEncoding.DecodeString(secret.Data["tls.crt"])
			if err != nil {
				return err
			}
			key, err := b64.StdEncoding.DecodeString(secret.Data["tls.key"])
			if err != nil {
				return err
			}
			cert, err := tls.X509KeyPair([]byte(crt), []byte(key))
			if err != nil {
				return err
			}
			ic.certificateRegistry.SyncCert(fmt.Sprintf("%s/%s", secret.Meta.Name, secret.Meta.Namespace), hosts, &cert)
			break
		}
	}

	if !found {
		log.Errorf("failed to find secret %s in namespace %s", secretName, ns)
		return err
	}
	
	return nil
}

// convert logs if an invalid found, but proceeds with the
// valid ones.  Reporting failures in Ingress status is not possible,
// because Ingress status field is v1beta1.LoadBalancerIngress that only
// supports IP and Hostname as string.
func (ing *ingress) convert(state *clusterState, df defaultFilters, r *certregistry.CertRegistry) ([]*eskip.Route, error) {
	var ewIngInfo map[string][]string // r.Id -> {namespace, name}
	if ing.kubernetesEnableEastWest {
		ewIngInfo = make(map[string][]string)
	}
	routes := make([]*eskip.Route, 0, len(state.ingresses))
	hostRoutes := make(map[string][]*eskip.Route)
	redirect := createRedirectInfo(ing.provideHTTPSRedirect, ing.httpsRedirectCode)
	if ing.ingressV1 {
		for _, i := range state.ingressesV1 {
			r, err := ing.ingressV1Route(i, redirect, state, hostRoutes, df, r)
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

	} else {
		for _, i := range state.ingresses {
			r, err := ing.ingressRoute(i, redirect, state, hostRoutes, df)
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
	}

	for host, rs := range hostRoutes {
		if len(rs) == 0 {
			continue
		}

		applyEastWestRange(ing.eastWestRangeDomains, ing.eastWestRangePredicates, host, rs)
		routes = append(routes, rs...)

		// if routes were configured, but there is no catchall route
		// defined for the host name, create a route which returns 404
		if !hasCatchAllRoutes(rs) {
			routes = append(routes, ing.addCatchAllRoutes(host, rs[0], redirect)...)
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
		log.Infof("enabled east west routes: %d %d %d %d", l, len(routes), len(ewroutes), len(hostRoutes))
	}

	return routes, nil
}
