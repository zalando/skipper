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
	IngressesClusterURI        = "/apis/extensions/v1beta1/ingresses"
	IngressesV1ClusterURI      = "/apis/networking.k8s.io/v1/ingresses"
	ZalandoResourcesClusterURI = "/apis/zalando.org/v1"
	RouteGroupsName            = "routegroups"
	routeGroupsClusterURI      = "/apis/zalando.org/v1/routegroups"
	routeGroupClassKey         = "zalando.org/routegroup.class"
	ServicesClusterURI         = "/api/v1/services"
	EndpointsClusterURI        = "/api/v1/endpoints"
	SecretsClusterURI          = "/api/v1/secrets"
	defaultKubernetesURL       = "http://localhost:8001"
	IngressesNamespaceFmt      = "/apis/extensions/v1beta1/namespaces/%s/ingresses"
	IngressesV1NamespaceFmt    = "/apis/networking.k8s.io/v1/namespaces/%s/ingresses"
	routeGroupsNamespaceFmt    = "/apis/zalando.org/v1/namespaces/%s/routegroups"
	ServicesNamespaceFmt       = "/api/v1/namespaces/%s/services"
	EndpointsNamespaceFmt      = "/api/v1/namespaces/%s/endpoints"
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
	secretsURI          string
	tokenProvider       secrets.SecretsProvider
	apiURL              string
	certificateRegistry *certregistry.CertRegistry

	routeGroupClass *regexp.Regexp
	ingressClass    *regexp.Regexp
	httpClient      *http.Client
	ingressV1       bool

	ingressLabelSelectors     string
	servicesLabelSelectors    string
	endpointsLabelSelectors   string
	secretsLabelSelectors     string
	routeGroupsLabelSelectors string

	loggedMissingRouteGroups bool
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
		for {
			select {
			case <-time.After(10 * time.Second):
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

	ingressURI := IngressesClusterURI
	if o.KubernetesIngressV1 {
		ingressURI = IngressesV1ClusterURI
	}
	c := &clusterClient{
		ingressV1:                 o.KubernetesIngressV1,
		ingressesURI:              ingressURI,
		routeGroupsURI:            routeGroupsClusterURI,
		servicesURI:               ServicesClusterURI,
		endpointsURI:              EndpointsClusterURI,
		secretsURI:                SecretsClusterURI,
		ingressClass:              ingClsRx,
		ingressLabelSelectors:     toLabelSelectorQuery(o.IngressLabelSelectors),
		servicesLabelSelectors:    toLabelSelectorQuery(o.ServicesLabelSelectors),
		endpointsLabelSelectors:   toLabelSelectorQuery(o.EndpointsLabelSelectors),
		secretsLabelSelectors:     toLabelSelectorQuery(o.SecretsLabelSelectors),
		routeGroupsLabelSelectors: toLabelSelectorQuery(o.RouteGroupsLabelSelectors),
		routeGroupClass:           rgClsRx,
		httpClient:                httpClient,
		apiURL:                    apiURL,
		certificateRegistry:       o.CertificateRegistry,
	}

	if o.KubernetesInCluster {
		c.tokenProvider = secrets.NewSecretPaths(time.Minute)
		err := c.tokenProvider.Add(serviceAccountDir + serviceAccountTokenKey)
		if err != nil {
			log.Errorf("Failed to Add secret %s: %v", serviceAccountDir+serviceAccountTokenKey, err)
			return nil, err
		}

		b, ok := c.tokenProvider.GetSecret(serviceAccountDir + serviceAccountTokenKey)
		if !ok {
			return nil, fmt.Errorf("failed to GetSecret: %s", serviceAccountDir+serviceAccountTokenKey)
		}
		log.Debugf("Got secret %d bytes", len(b))
	}

	if o.KubernetesNamespace != "" {
		c.setNamespace(o.KubernetesNamespace)
	}

	return c, nil
}

// serializes a given map of label selectors to a string that can be appended to a request URI to kubernetes
// Examples (note that the resulting value in the query is URL escaped, for readability this is not done in examples):
// 	[] becomes ``
// 	["label": ""] becomes `?labelSelector=label`
// 	["label": "value"] becomes `?labelSelector=label=value`
// 	["label": "value", "label2": "value2"] becomes `?labelSelector=label=value&label2=value2`
func toLabelSelectorQuery(selectors map[string]string) string {
	if selectors == nil || len(selectors) == 0 {
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
	if c.ingressV1 {
		c.ingressesURI = fmt.Sprintf(IngressesV1NamespaceFmt, namespace)
	} else {
		c.ingressesURI = fmt.Sprintf(IngressesNamespaceFmt, namespace)
	}
	c.routeGroupsURI = fmt.Sprintf(routeGroupsNamespaceFmt, namespace)
	c.servicesURI = fmt.Sprintf(ServicesNamespaceFmt, namespace)
	c.endpointsURI = fmt.Sprintf(EndpointsNamespaceFmt, namespace)
	c.secretsURI = fmt.Sprintf(SecretsNamespaceFmt, namespace)
}

func (c *clusterClient) createRequest(uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest("GET", c.apiURL+uri, body)
	if err != nil {
		return nil, err
	}

	if c.tokenProvider != nil {
		token, ok := c.tokenProvider.GetSecret(serviceAccountDir + serviceAccountTokenKey)
		if !ok {
			return nil, fmt.Errorf("secret not found: %v", serviceAccountDir+serviceAccountTokenKey)
		}
		req.Header.Set("Authorization", "Bearer "+string(token))
	}

	return req, nil
}

func (c *clusterClient) getJSON(uri string, a interface{}) error {
	log.Debugf("making request to: %s", uri)

	req, err := c.createRequest(uri, nil)
	if err != nil {
		return err
	}

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("request to %s failed: %v", uri, err)
		return err
	}

	log.Debugf("request to %s succeeded", uri)
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusNotFound {
		return errResourceNotFound
	}

	if rsp.StatusCode != http.StatusOK {
		log.Debugf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return fmt.Errorf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err = io.Copy(b, rsp.Body); err != nil {
		log.Debugf("reading response body failed: %v", err)
		return err
	}

	err = json.Unmarshal(b.Bytes(), a)
	if err != nil {
		log.Debugf("invalid response format: %v", err)
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

// filterIngressesByClass will filter only the ingresses that have the valid class, these are
// the defined one, empty string class or not class at all
func (c *clusterClient) filterIngressesByClass(items []*definitions.IngressItem) []*definitions.IngressItem {
	validIngs := []*definitions.IngressItem{}

	for _, ing := range items {
		if c.ingressClassMissmatch(ing.Metadata) {
			continue
		}
		validIngs = append(validIngs, ing)
	}

	return validIngs
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

func (c *clusterClient) loadIngresses() ([]*definitions.IngressItem, error) {
	var il definitions.IngressList
	if err := c.getJSON(c.ingressesURI+c.ingressLabelSelectors, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	fItems := c.filterIngressesByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))
	sortByMetadata(fItems, func(i int) *definitions.Metadata { return fItems[i].Metadata })
	return fItems, nil
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
	return fItems, nil
}

func (c *clusterClient) LoadRouteGroups() ([]*definitions.RouteGroupItem, error) {
	var rgl definitions.RouteGroupList
	if err := c.getJSON(c.routeGroupsURI+c.routeGroupsLabelSelectors, &rgl); err != nil {
		return nil, err
	}

	rgs := make([]*definitions.RouteGroupItem, 0, len(rgl.Items))
	for _, i := range rgl.Items {
		// Validate RouteGroup item.
		if err := definitions.ValidateRouteGroup(i); err != nil {
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
		ingresses   []*definitions.IngressItem
		secrets     map[definitions.ResourceID]*secret
	)
	if c.ingressV1 {
		ingressesV1, err = c.loadIngressesV1()
	} else {
		ingresses, err = c.loadIngresses()
	}
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

	endpoints, err := c.loadEndpoints()
	if err != nil {
		return nil, err
	}

	if c.certificateRegistry != nil {
		secrets, err = c.loadSecrets()
		if err != nil {
			return nil, err
		}
	}

	return &clusterState{
		ingresses:       ingresses,
		ingressesV1:     ingressesV1,
		routeGroups:     routeGroups,
		services:        services,
		endpoints:       endpoints,
		secrets:         secrets,
		cachedEndpoints: make(map[endpointID][]string),
	}, nil
}
