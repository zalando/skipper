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
	"os"
	"regexp"
	"sort"
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
	FabricGatewaysName         = "fabricgateways"
	fabricGatewaysClusterURI   = ZalandoResourcesClusterURI + "/fabricgateways"
	fabricGatewayClassKey      = "zalando.org/fabricgateway.class"
	RouteGroupsName            = "routegroups"
	routeGroupsClusterURI      = ZalandoResourcesClusterURI + "/routegroups"
	routeGroupClassKey         = "zalando.org/routegroup.class"
	StacksetsName              = "stacksets"
	stacksetsClusterURI        = ZalandoResourcesClusterURI + "/routegroups"
	ServicesClusterURI         = "/api/v1/services"
	EndpointsClusterURI        = "/api/v1/endpoints"
	SecretsClusterURI          = "/api/v1/secrets"
	defaultKubernetesURL       = "http://localhost:8001"
	IngressesNamespaceFmt      = "/apis/extensions/v1beta1/namespaces/%s/ingresses"
	IngressesV1NamespaceFmt    = "/apis/networking.k8s.io/v1/namespaces/%s/ingresses"
	fabricGatewaysNamespaceFmt = ZalandoResourcesClusterURI + "/namespaces/%s/fabricgateways"
	routeGroupsNamespaceFmt    = ZalandoResourcesClusterURI + "namespaces/%s/routegroups"
	stacksetsNamespaceFmt      = ZalandoResourcesClusterURI + "namespaces/%s/stacksets"
	ServicesNamespaceFmt       = "/api/v1/namespaces/%s/services"
	EndpointsNamespaceFmt      = "/api/v1/namespaces/%s/endpoints"
	SecretsNamespaceFmt        = "/api/v1/namespaces/%s/secrets"
	serviceAccountDir          = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey     = "token"
	serviceAccountRootCAKey    = "ca.crt"
)

const (
	RouteGroupsNotInstalledMessage = `RouteGroups CRD is not installed in the cluster.
See: https://opensource.zalando.com/skipper/kubernetes/routegroups/#installation`
	FabricGatewaysNotInstalledMessage = `FabricGateways CRD is not installed in the cluster.
See: https://opensource.zalando.com/skipper/kubernetes/fabricgateways/#installation`
	StacksetsNotInstalledMessage = `Stacksets CRD is not installed in the cluster, so you can not use x-external-service-provider in case you have FabricGateways installed.
See: https://opensource.zalando.com/skipper/kubernetes/fabricgateways/#installation`
)

type clusterClient struct {
	ingressesURI        string
	fabricGatewaysURI   string
	routeGroupsURI      string
	stacksetsURI        string
	servicesURI         string
	endpointsURI        string
	secretsURI          string
	tokenProvider       secrets.SecretsProvider
	apiURL              string
	certificateRegistry *certregistry.CertRegistry

	fabricGatewayClass *regexp.Regexp
	routeGroupClass    *regexp.Regexp
	ingressClass       *regexp.Regexp
	httpClient         *http.Client
	ingressV1          bool

	loggedMissingFabricGateways bool
	loggedMissingStacksets      bool
	loggedMissingRouteGroups    bool
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

func newClusterClient(o Options, apiURL, ingCls, fgCls, rgCls string, quit <-chan struct{}) (*clusterClient, error) {
	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, o.KubernetesInCluster, quit)
	if err != nil {
		return nil, err
	}

	ingClsRx, err := regexp.Compile(ingCls)
	if err != nil {
		return nil, err
	}

	fgClsRx, err := regexp.Compile(fgCls)
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
		ingressV1:           o.KubernetesIngressV1,
		ingressesURI:        ingressURI,
		fabricGatewaysURI:   fabricGatewaysClusterURI,
		routeGroupsURI:      routeGroupsClusterURI,
		stacksetsURI:        stacksetsClusterURI,
		servicesURI:         ServicesClusterURI,
		endpointsURI:        EndpointsClusterURI,
		secretsURI:          SecretsClusterURI,
		ingressClass:        ingClsRx,
		fabricGatewayClass:  fgClsRx,
		routeGroupClass:     rgClsRx,
		httpClient:          httpClient,
		apiURL:              apiURL,
		certificateRegistry: o.CertificateRegistry,
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

func (c *clusterClient) setNamespace(namespace string) {
	if c.ingressV1 {
		c.ingressesURI = fmt.Sprintf(IngressesV1NamespaceFmt, namespace)
	} else {
		c.ingressesURI = fmt.Sprintf(IngressesNamespaceFmt, namespace)
	}
	c.fabricGatewaysURI = fmt.Sprintf(fabricGatewaysNamespaceFmt, namespace)
	c.routeGroupsURI = fmt.Sprintf(routeGroupsNamespaceFmt, namespace)
	c.stacksetsURI = fmt.Sprintf(stacksetsNamespaceFmt, namespace)
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

func (c *clusterClient) clusterHasFabricGateways() (bool, error) {
	var crl ClusterResourceList
	if err := c.getJSON(ZalandoResourcesClusterURI, &crl); err != nil { // it probably should bounce once
		return false, err
	}

	for _, cr := range crl.Items {
		if cr.Name == FabricGatewaysName {
			return true, nil
		}
	}

	return false, nil
}

func (c *clusterClient) clusterHasStacksets() (bool, error) {
	var crl ClusterResourceList
	if err := c.getJSON(ZalandoResourcesClusterURI, &crl); err != nil { // it probably should bounce once
		return false, err
	}

	for _, cr := range crl.Items {
		if cr.Name == StacksetsName {
			return true, nil
		}
	}

	return false, nil
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
	if err := c.getJSON(c.ingressesURI, &il); err != nil {
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
	if err := c.getJSON(c.ingressesURI, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	fItems := c.filterIngressesV1ByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))
	sortByMetadata(fItems, func(i int) *definitions.Metadata { return fItems[i].Metadata })
	return fItems, nil
}

func (c *clusterClient) LoadFabricgateways() ([]*definitions.FabricItem, error) {
	var fl definitions.FabricList
	err := c.getJSON(c.fabricGatewaysURI, &fl)
	if err != nil {
		return nil, err
	}

	fcs := make([]*definitions.FabricItem, 0, len(fl.Items))
	for _, fg := range fl.Items {
		err := definitions.ValidateFabricResource(fg)
		if err != nil {
			log.Errorf("Failed to validate FabricGateway resource: %v", err)
			continue
		}

		fcs = append(fcs, fg)
	}

	return fcs, nil
}

// LoadStacksetsTraffic returns a map of definitions.ResourceID to slice of
// ActualTraffic, used by the FabricGateway x-external-service-provider feature
func (c *clusterClient) LoadStacksetsTraffic() (map[definitions.ResourceID][]*definitions.ActualTraffic, error) {
	type (
		status struct {
			Traffic []*definitions.ActualTraffic `json:"traffic"`
		}
		stackset struct {
			Meta   *definitions.Metadata `json:"metadata"`
			Status *status               `json:"status"`
		}
		stacksetList struct {
			Items []*stackset `json:"items"`
		}
	)

	var stl stacksetList
	err := c.getJSON(c.stacksetsURI, &stl)
	if err != nil {
		return nil, err
	}

	fcs := make(map[definitions.ResourceID][]*definitions.ActualTraffic)
	for _, st := range stl.Items {
		rid := definitions.ResourceID{
			Namespace: st.Meta.Namespace,
			Name:      st.Meta.Name,
		}
		fcs[rid] = st.Status.Traffic
	}
	return fcs, nil
}

func (c *clusterClient) LoadRouteGroups() ([]*definitions.RouteGroupItem, error) {
	var rgl definitions.RouteGroupList
	if err := c.getJSON(c.routeGroupsURI, &rgl); err != nil {
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
	if err := c.getJSON(c.servicesURI, &services); err != nil {
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
	if err := c.getJSON(c.secretsURI, &secrets); err != nil {
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
	if err := c.getJSON(c.endpointsURI, &endpoints); err != nil {
		log.Debugf("requesting all endpoints failed: %v", err)
		return nil, err
	}

	log.Debugf("all endpoints received: %d", len(endpoints.Items))
	result := make(map[definitions.ResourceID]*endpoint)
	for _, endpoint := range endpoints.Items {
		result[endpoint.Meta.ToResourceID()] = endpoint
	}

	return result, nil
}

func (c *clusterClient) logMissingFabricGatewaysOnce() {
	if c.loggedMissingFabricGateways {
		return
	}

	c.loggedMissingFabricGateways = true
	log.Warn(FabricGatewaysNotInstalledMessage)
}

func (c *clusterClient) logMissingStacksetsOnce() {
	if c.loggedMissingStacksets {
		return
	}

	c.loggedMissingStacksets = true
	log.Warn(StacksetsNotInstalledMessage)
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

	var (
		fabricGateways   []*definitions.FabricItem
		stacksetsTraffic map[definitions.ResourceID][]*definitions.ActualTraffic
	)
	if hasFabricGateways, err := c.clusterHasFabricGateways(); errors.Is(err, errResourceNotFound) {
		c.logMissingFabricGatewaysOnce()
	} else if err != nil {
		log.Errorf("Error while checking known resource types: %v.", err)
	} else if hasFabricGateways {
		c.loggedMissingRouteGroups = false
		if fabricGateways, err = c.LoadFabricgateways(); err != nil {
			return nil, err
		}

		// load stacksets only if fabricgateways are loaded successfully
		if hasStacksets, err := c.clusterHasStacksets(); errors.Is(err, errResourceNotFound) {
			c.logMissingStacksetsOnce()
		} else if err != nil {
			log.Errorf("Error while checking known resource types: %v.", err)
		} else if hasStacksets {
			c.loggedMissingStacksets = false
			if stacksetsTraffic, err = c.LoadStacksetsTraffic(); err != nil {
				return nil, err
			}
		}
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
		ingresses:        ingresses,
		ingressesV1:      ingressesV1,
		fabricGateways:   fabricGateways,
		stacksetsTraffic: stacksetsTraffic,
		routeGroups:      routeGroups,
		services:         services,
		endpoints:        endpoints,
		secrets:          secrets,
		cachedEndpoints:  make(map[endpointID][]string),
	}, nil

}
