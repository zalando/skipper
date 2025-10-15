package kubernetes

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/secrets"
	"github.com/zalando/skipper/secrets/certregistry"
)

const (
	ingressClassKey            = "kubernetes.io/ingress.class"
	IngressesV1ClusterURI      = "/apis/networking.k8s.io/v1/ingresses"
	ZalandoResourcesClusterURI = "/apis/zalando.org/v1"
	RouteGroupsName            = "routegroups"
	RouteGroupsClusterURI      = "/apis/zalando.org/v1/routegroups"
	routeGroupClassKey         = "zalando.org/routegroup.class"
	ServicesClusterURI         = "/api/v1/services"
	EndpointsClusterURI        = "/api/v1/endpoints"
	EndpointSlicesClusterURI   = "/apis/discovery.k8s.io/v1/endpointslices"
	SecretsClusterURI          = "/api/v1/secrets"
	defaultKubernetesURL       = "http://localhost:8001"
	IngressesV1NamespaceFmt    = "/apis/networking.k8s.io/v1/namespaces/%s/ingresses"
	RouteGroupsNamespaceFmt    = "/apis/zalando.org/v1/namespaces/%s/routegroups"
	ServicesNamespaceFmt       = "/api/v1/namespaces/%s/services"
	EndpointsNamespaceFmt      = "/api/v1/namespaces/%s/endpoints"
	EndpointSlicesNamespaceFmt = "/apis/discovery.k8s.io/v1/namespaces/%s/endpointslices"
	SecretsNamespaceFmt        = "/api/v1/namespaces/%s/secrets"
	serviceAccountDir          = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey     = "token"
	serviceAccountRootCAKey    = "ca.crt"
	labelSelectorFmt           = "%s=%s"
	labelSelectorQueryFmt      = "?labelSelector=%s"
)

const RouteGroupsNotInstalledMessage = `RouteGroups CRD is not installed in the cluster.
See: https://opensource.zalando.com/skipper/kubernetes/routegroups/#installation`

type clusterClient struct {
	ingressesURI        string
	routeGroupsURI      string
	servicesURI         string
	endpointsURI        string
	endpointSlicesURI   string
	secretsURI          string
	tokenProvider       secrets.SecretsProvider
	tokenFile           string
	apiURL              string
	certificateRegistry *certregistry.CertRegistry

	routeGroupClass *regexp.Regexp
	ingressClass    *regexp.Regexp
	httpClient      *http.Client

	ingressLabelSelectors        string
	servicesLabelSelectors       string
	endpointsLabelSelectors      string
	endpointSlicesLabelSelectors string
	secretsLabelSelectors        string
	routeGroupsLabelSelectors    string

	enableEndpointSlices bool

	loggedMissingRouteGroups bool
	routeGroupValidator      *definitions.RouteGroupValidator
	ingressValidator         *definitions.IngressV1Validator
}

var (
	errResourceNotFound     = errors.New("resource not found")
	errServiceNotFound      = errors.New("service not found")
	errAPIServerURLNotFound = errors.New("kubernetes API server URL could not be constructed from env vars")
	errInvalidCertificate   = errors.New("invalid CA")
)

func buildHTTPClient(certFilePath string, inCluster bool, quit <-chan struct{}) (*http.Client, error) {
	if !inCluster {
		return http.DefaultClient, nil
	}

	rootCA, err := os.ReadFile(certFilePath)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(rootCA) {
		return nil, errInvalidCertificate
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 30 * time.Second,
		MaxIdleConns:          5,
		MaxIdleConnsPerHost:   5,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certPool,
		},
	}

	// regularly force closing idle connections
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				transport.CloseIdleConnections()
			case <-quit:
				return
			}
		}
	}()

	return &http.Client{
		Transport: transport,
	}, nil
}

func newClusterClient(o Options, apiURL, ingCls, rgCls string, quit <-chan struct{}) (*clusterClient, error) {
	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, o.KubernetesInCluster, quit)
	if err != nil {
		return nil, err
	}

	ingClsRx, err := regexp.Compile(ingCls)
	if err != nil {
		return nil, err
	}

	rgClsRx, err := regexp.Compile(rgCls)
	if err != nil {
		return nil, err
	}

	c := &clusterClient{
		ingressesURI:                 IngressesV1ClusterURI,
		routeGroupsURI:               RouteGroupsClusterURI,
		servicesURI:                  ServicesClusterURI,
		endpointsURI:                 EndpointsClusterURI,
		endpointSlicesURI:            EndpointSlicesClusterURI,
		secretsURI:                   SecretsClusterURI,
		ingressClass:                 ingClsRx,
		ingressLabelSelectors:        toLabelSelectorQuery(o.IngressLabelSelectors),
		servicesLabelSelectors:       toLabelSelectorQuery(o.ServicesLabelSelectors),
		endpointsLabelSelectors:      toLabelSelectorQuery(o.EndpointsLabelSelectors),
		endpointSlicesLabelSelectors: toLabelSelectorQuery(o.EndpointSlicesLabelSelectors),
		secretsLabelSelectors:        toLabelSelectorQuery(o.SecretsLabelSelectors),
		routeGroupsLabelSelectors:    toLabelSelectorQuery(o.RouteGroupsLabelSelectors),
		routeGroupClass:              rgClsRx,
		httpClient:                   httpClient,
		apiURL:                       apiURL,
		certificateRegistry:          o.CertificateRegistry,
		routeGroupValidator:          &definitions.RouteGroupValidator{EnableAdvancedValidation: false},
		ingressValidator:             &definitions.IngressV1Validator{EnableAdvancedValidation: false},
		enableEndpointSlices:         o.KubernetesEnableEndpointslices,
	}

	if o.KubernetesInCluster {
		c.tokenProvider = secrets.NewSecretPaths(time.Minute)
		c.tokenFile = serviceAccountDir + serviceAccountTokenKey
	} else if o.TokenFile != "" {
		c.tokenProvider = secrets.NewSecretPaths(time.Minute)
		c.tokenFile = o.TokenFile
	}

	if c.tokenProvider != nil {
		if err := c.tokenProvider.Add(c.tokenFile); err != nil {
			return nil, fmt.Errorf("failed to add secret %s: %w", c.tokenFile, err)
		}

		if b, ok := c.tokenProvider.GetSecret(c.tokenFile); ok {
			log.Debugf("Got secret %d bytes from %s", len(b), c.tokenFile)
		} else {
			return nil, fmt.Errorf("failed to get secret %s", c.tokenFile)
		}
	}

	if o.KubernetesNamespace != "" {
		c.setNamespace(o.KubernetesNamespace)
	}

	return c, nil
}

// serializes a given map of label selectors to a string that can be appended to a request URI to kubernetes
// Examples (note that the resulting value in the query is URL escaped, for readability this is not done in examples):
//
//	[] becomes ``
//	["label": ""] becomes `?labelSelector=label`
//	["label": "value"] becomes `?labelSelector=label=value`
//	["label": "value", "label2": "value2"] becomes `?labelSelector=label=value&label2=value2`
func toLabelSelectorQuery(selectors map[string]string) string {
	if len(selectors) == 0 {
		return ""
	}

	var strs []string
	for k, v := range selectors {
		if v == "" {
			strs = append(strs, k)
		} else {
			strs = append(strs, fmt.Sprintf(labelSelectorFmt, k, v))
		}
	}

	return fmt.Sprintf(labelSelectorQueryFmt, url.QueryEscape(strings.Join(strs, ",")))
}

func (c *clusterClient) setNamespace(namespace string) {
	c.ingressesURI = fmt.Sprintf(IngressesV1NamespaceFmt, namespace)
	c.routeGroupsURI = fmt.Sprintf(RouteGroupsNamespaceFmt, namespace)
	c.servicesURI = fmt.Sprintf(ServicesNamespaceFmt, namespace)
	c.endpointsURI = fmt.Sprintf(EndpointsNamespaceFmt, namespace)
	c.endpointSlicesURI = fmt.Sprintf(EndpointSlicesNamespaceFmt, namespace)
	c.secretsURI = fmt.Sprintf(SecretsNamespaceFmt, namespace)
}

func (c *clusterClient) createRequest(uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest("GET", c.apiURL+uri, body)
	if err != nil {
		return nil, err
	}

	if c.tokenProvider != nil {
		token, ok := c.tokenProvider.GetSecret(c.tokenFile)
		if !ok {
			return nil, fmt.Errorf("secret not found: %v", c.tokenFile)
		}
		req.Header.Set("Authorization", "Bearer "+string(token))
	}

	return req, nil
}

func (c *clusterClient) getJSON(uri string, a interface{}) error {
	log.Tracef("making request to: %s", uri)

	req, err := c.createRequest(uri, nil)
	if err != nil {
		return err
	}

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		log.Tracef("request to %s failed: %v", uri, err)
		return err
	}

	log.Tracef("request to %s succeeded", uri)
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusNotFound {
		return errResourceNotFound
	}

	if rsp.StatusCode != http.StatusOK {
		log.Tracef("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return fmt.Errorf("request to %s failed, status: %d, %s", uri, rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err = io.Copy(b, rsp.Body); err != nil {
		log.Tracef("reading response body failed: %v", err)
		return err
	}

	err = json.Unmarshal(b.Bytes(), a)
	if err != nil {
		log.Tracef("invalid response format: %v", err)
	}

	return err
}

func (c *clusterClient) clusterHasRouteGroups() (bool, error) {
	var crl ClusterResourceList
	if err := c.getJSON(ZalandoResourcesClusterURI, &crl); err != nil { // it probably should bounce once
		return false, err
	}

	for _, cr := range crl.Items {
		if cr.Name == RouteGroupsName {
			return true, nil
		}
	}

	return false, nil
}

func (c *clusterClient) ingressClassMissmatch(m *definitions.Metadata) bool {
	// No Metadata is the same as no annotations for us
	if m != nil {
		cls, ok := m.Annotations[ingressClassKey]
		// Skip loop iteration if not valid ingress (non defined, empty or non defined one)
		return ok && cls != "" && !c.ingressClass.MatchString(cls)
	}
	return false
}

// filterIngressesV1ByClass will filter only the ingresses that have the valid class, these are
// the defined one, empty string class or not class at all
func (c *clusterClient) filterIngressesV1ByClass(items []*definitions.IngressV1Item) []*definitions.IngressV1Item {
	validIngs := []*definitions.IngressV1Item{}

	for _, ing := range items {
		// v1beta1 style
		if c.ingressClassMissmatch(ing.Metadata) {
			continue
		}

		// v1 style, TODO(sszuecs) we need also to fetch ingressclass object and check what should be done
		if ing.Spec == nil || ing.Spec.IngressClassName == "" || c.ingressClass.MatchString(ing.Spec.IngressClassName) {
			validIngs = append(validIngs, ing)
		}
	}

	return validIngs
}

func sortByMetadata(slice interface{}, getMetadata func(int) *definitions.Metadata) {
	sort.Slice(slice, func(i, j int) bool {
		mI := getMetadata(i)
		mJ := getMetadata(j)
		if mI == nil && mJ != nil {
			return true
		} else if mJ == nil {
			return false
		}
		nsI := mI.Namespace
		nsJ := mJ.Namespace
		if nsI != nsJ {
			return nsI < nsJ
		}
		return mI.Name < mJ.Name
	})
}

func (c *clusterClient) loadIngressesV1() ([]*definitions.IngressV1Item, error) {
	var il definitions.IngressV1List
	if err := c.getJSON(c.ingressesURI+c.ingressLabelSelectors, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}
	log.Debugf("all ingresses received: %d", len(il.Items))

	fItems := c.filterIngressesV1ByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))

	sortByMetadata(fItems, func(i int) *definitions.Metadata { return fItems[i].Metadata })

	validatedItems := make([]*definitions.IngressV1Item, 0, len(fItems))
	for _, i := range fItems {
		if err := c.ingressValidator.Validate(i); err != nil {
			log.Errorf("[ingress] %v", err)
			continue
		}
		validatedItems = append(validatedItems, i)
	}

	return validatedItems, nil
}

func (c *clusterClient) LoadRouteGroups() ([]*definitions.RouteGroupItem, error) {
	var rgl definitions.RouteGroupList
	if err := c.getJSON(c.routeGroupsURI+c.routeGroupsLabelSelectors, &rgl); err != nil {
		return nil, err
	}
	log.Debugf("all routegroups received: %d", len(rgl.Items))

	rgs := make([]*definitions.RouteGroupItem, 0, len(rgl.Items))
	for _, i := range rgl.Items {
		// Validate RouteGroup item.
		if err := c.routeGroupValidator.Validate(i); err != nil {
			log.Errorf("[routegroup] %v", err)
			continue
		}

		// Check the RouteGroup has a valid class annotation.
		// Not defined, or empty are ok too.
		if i.Metadata != nil {
			cls, ok := i.Metadata.Annotations[routeGroupClassKey]
			if ok && cls != "" && !c.routeGroupClass.MatchString(cls) {
				continue
			}
		}

		rgs = append(rgs, i)
	}

	log.Debugf("filtered valid routegroups by routegroups class: %d", len(rgs))

	sortByMetadata(rgs, func(i int) *definitions.Metadata { return rgs[i].Metadata })

	return rgs, nil
}

func (c *clusterClient) loadServices() (map[definitions.ResourceID]*service, error) {
	var services serviceList
	if err := c.getJSON(c.servicesURI+c.servicesLabelSelectors, &services); err != nil {
		log.Debugf("requesting all services failed: %v", err)
		return nil, err
	}
	log.Debugf("all services received: %d", len(services.Items))

	result := make(map[definitions.ResourceID]*service)
	var hasInvalidService bool
	for _, service := range services.Items {
		if service == nil || service.Meta == nil || service.Spec == nil {
			hasInvalidService = true
			continue
		}

		result[service.Meta.ToResourceID()] = service
	}

	if hasInvalidService {
		log.Errorf("Invalid service resource detected.")
	}

	return result, nil
}

func (c *clusterClient) loadSecrets() (map[definitions.ResourceID]*secret, error) {
	var secrets secretList
	if err := c.getJSON(c.secretsURI+c.secretsLabelSelectors, &secrets); err != nil {
		log.Debugf("requesting all secrets failed: %v", err)
		return nil, err
	}
	log.Debugf("all secrets received: %d", len(secrets.Items))

	result := make(map[definitions.ResourceID]*secret)
	for _, secret := range secrets.Items {
		if secret == nil || secret.Metadata == nil {
			continue
		}

		result[secret.Metadata.ToResourceID()] = secret
	}

	return result, nil
}

func (c *clusterClient) loadEndpoints() (map[definitions.ResourceID]*endpoint, error) {
	var endpoints endpointList
	if err := c.getJSON(c.endpointsURI+c.endpointsLabelSelectors, &endpoints); err != nil {
		log.Debugf("requesting all endpoints failed: %v", err)
		return nil, err
	}
	log.Debugf("all endpoints received: %d", len(endpoints.Items))

	result := make(map[definitions.ResourceID]*endpoint)
	for _, endpoint := range endpoints.Items {
		resID := endpoint.Meta.ToResourceID()
		result[resID] = endpoint
	}

	return result, nil
}

// loadEndpointSlices is different from the other load$Kind()
// functions because there are 1..N endpointslices created for a given
// service. Endpointslices need to be deduplicated and state needs to
// be checked. We read all endpointslices and create de-duplicated
// business objects [skipperEndpointSlice] instead of raw Kubernetes
// objects, because we need just a clean list of load balancer
// members. The returned map will return the full list of ready
// non-terminating endpoints that should be in the load balancer of a
// given service, check [endpointSlice.ToResourceID].
func (c *clusterClient) loadEndpointSlices() (map[definitions.ResourceID]*skipperEndpointSlice, error) {
	var endpointSlices endpointSliceList
	if err := c.getJSON(c.endpointSlicesURI+c.endpointSlicesLabelSelectors, &endpointSlices); err != nil {
		log.Debugf("requesting all endpointslices failed: %v", err)
		return nil, err
	}
	log.Debugf("all endpointslices received: %d", len(endpointSlices.Items))

	return collectReadyEndpoints(&endpointSlices), nil
}

func collectReadyEndpoints(endpointSlices *endpointSliceList) map[definitions.ResourceID]*skipperEndpointSlice {
	mapSlices := make(map[definitions.ResourceID][]*endpointSlice)
	for _, endpointSlice := range endpointSlices.Items {
		// https://github.com/zalando/skipper/issues/3151
		// endpointslices can have nil ports
		if endpointSlice.Ports != nil {
			resID := endpointSlice.ToResourceID() // service resource ID
			mapSlices[resID] = append(mapSlices[resID], endpointSlice)
		}
	}

	result := make(map[definitions.ResourceID]*skipperEndpointSlice)
	for resID, epSlices := range mapSlices {
		if len(epSlices) == 0 {
			continue
		}

		result[resID] = &skipperEndpointSlice{
			Meta: epSlices[0].Meta,
		}

		terminatingEps := make(map[string]struct{})
		resEps := make(map[string]*skipperEndpoint)

		for i := range epSlices {

			for _, ep := range epSlices[i].Endpoints {
				// Addresses [1..100] of the same AddressType, as kube-proxy we use the first
				// see also https://github.com/kubernetes/kubernetes/issues/106267
				address := ep.Addresses[0]
				if _, ok := terminatingEps[address]; ok {
					// already known terminating
				} else if ep.isTerminating() {
					terminatingEps[address] = struct{}{}
					// if we had this one with a non terminating condition,
					// we should delete it, because of eventual consistency
					// it is actually terminating
					delete(resEps, address)
				} else if ep.Conditions == nil {
					// if conditions are nil then we need to treat is as ready
					resEps[address] = &skipperEndpoint{
						Address: address,
						Zone:    ep.Zone,
					}
				} else if ep.isReady() {
					resEps[address] = &skipperEndpoint{
						Address: address,
						Zone:    ep.Zone,
					}
				}
			}

			result[resID].Ports = epSlices[i].Ports
		}
		for _, o := range resEps {
			result[resID].Endpoints = append(result[resID].Endpoints, o)
		}
	}
	return result
}

// loadEndpointAddresses returns the list of all addresses for the given service using endpoints or endpointslices API.
func (c *clusterClient) loadEndpointAddresses(namespace, name string) ([]string, error) {
	var result []string
	if c.enableEndpointSlices {
		url := fmt.Sprintf(EndpointSlicesNamespaceFmt, namespace) +
			toLabelSelectorQuery(map[string]string{endpointSliceServiceNameLabel: name})

		var endpointSlices endpointSliceList
		if err := c.getJSON(url, &endpointSlices); err != nil {
			return nil, fmt.Errorf("requesting endpointslices for %s/%s failed: %w", namespace, name, err)
		}

		ready := collectReadyEndpoints(&endpointSlices)
		if len(ready) != 1 {
			return nil, fmt.Errorf("unexpected number of endpoint slices for %s/%s: %d", namespace, name, len(ready))
		}

		for _, eps := range ready {
			result = eps.addresses()
			break
		}
	} else {
		url := fmt.Sprintf(EndpointsNamespaceFmt, namespace) + "/" + name

		var ep endpoint
		if err := c.getJSON(url, &ep); err != nil {
			return nil, fmt.Errorf("requesting endpoints for %s/%s failed: %w", namespace, name, err)
		}
		result = ep.addresses()
	}
	sort.Strings(result)

	return result, nil
}

func (c *clusterClient) logMissingRouteGroupsOnce() {
	if c.loggedMissingRouteGroups {
		return
	}

	c.loggedMissingRouteGroups = true
	log.Warn(RouteGroupsNotInstalledMessage)
}

func (c *clusterClient) fetchClusterState() (*clusterState, error) {
	var (
		err         error
		ingressesV1 []*definitions.IngressV1Item
	)
	ingressesV1, err = c.loadIngressesV1()
	if err != nil {
		return nil, err
	}

	var routeGroups []*definitions.RouteGroupItem
	if hasRouteGroups, err := c.clusterHasRouteGroups(); errors.Is(err, errResourceNotFound) {
		c.logMissingRouteGroupsOnce()
	} else if err != nil {
		log.Errorf("Error while checking known resource types: %v.", err)
	} else if hasRouteGroups {
		c.loggedMissingRouteGroups = false
		if routeGroups, err = c.LoadRouteGroups(); err != nil {
			return nil, err
		}
	}

	services, err := c.loadServices()
	if err != nil {
		return nil, err
	}

	state := &clusterState{
		ingressesV1:          ingressesV1,
		routeGroups:          routeGroups,
		services:             services,
		cachedEndpoints:      make(map[endpointID][]string),
		enableEndpointSlices: c.enableEndpointSlices,
	}

	if c.enableEndpointSlices {
		state.endpointSlices, err = c.loadEndpointSlices()
		if err != nil {
			return nil, err
		}
	} else {
		state.endpoints, err = c.loadEndpoints()
		if err != nil {
			return nil, err
		}
	}

	if c.certificateRegistry != nil {
		state.secrets, err = c.loadSecrets()
		if err != nil {
			return nil, err
		}
	}

	return state, nil
}
