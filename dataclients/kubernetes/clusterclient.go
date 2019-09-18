package kubernetes

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	ingressClassKey         = "kubernetes.io/ingress.class"
	ingressesClusterURI     = "/apis/extensions/v1beta1/ingresses"
	servicesClusterURI      = "/api/v1/services"
	endpointsClusterURI     = "/api/v1/endpoints"
	defaultKubernetesURL    = "http://localhost:8001"
	ingressesNamespaceFmt   = "/apis/extensions/v1beta1/namespaces/%s/ingresses"
	servicesNamespaceFmt    = "/api/v1/namespaces/%s/services"
	endpointsNamespaceFmt   = "/api/v1/namespaces/%s/endpoints"
	serviceAccountDir       = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey  = "token"
	serviceAccountRootCAKey = "ca.crt"
)

type clusterClient struct {
	ingressesURI string
	servicesURI  string
	endpointsURI string
	ingressClass *regexp.Regexp
	token        string
	httpClient   *http.Client
	apiURL       string
}

var (
	errResourceNotFound     = errors.New("resource not found")
	errServiceNotFound      = errors.New("service not found")
	errEndpointNotFound     = errors.New("endpoint not found")
	errAPIServerURLNotFound = errors.New("kubernetes API server URL could not be constructed from env vars")
	errInvalidCertificate   = errors.New("invalid CA")
)

func buildHTTPClient(certFilePath string, inCluster bool, quit <-chan struct{}) (*http.Client, error) {
	if !inCluster {
		return http.DefaultClient, nil
	}

	rootCA, err := ioutil.ReadFile(certFilePath)
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

func readServiceAccountToken(tokenFilePath string, inCluster bool) (string, error) {
	if !inCluster {
		return "", nil
	}

	bToken, err := ioutil.ReadFile(tokenFilePath)
	if err != nil {
		return "", err
	}

	return string(bToken), nil
}

func newClusterClient(o Options, apiURL, ingCls string, quit <-chan struct{}) (*clusterClient, error) {
	token, err := readServiceAccountToken(serviceAccountDir+serviceAccountTokenKey, o.KubernetesInCluster)
	if err != nil {
		return nil, err
	}

	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, o.KubernetesInCluster, quit)
	if err != nil {
		return nil, err
	}

	ingClsRx, err := regexp.Compile(ingCls)
	if err != nil {
		return nil, err
	}

	c := &clusterClient{
		ingressesURI: ingressesClusterURI,
		servicesURI:  servicesClusterURI,
		endpointsURI: endpointsClusterURI,
		ingressClass: ingClsRx,
		httpClient:   httpClient,
		token:        token,
		apiURL:       apiURL,
	}

	if o.KubernetesNamespace != "" {
		c.setNamespace(o.KubernetesNamespace)
	}

	return c, nil
}

func (c *clusterClient) setNamespace(namespace string) {
	c.ingressesURI = fmt.Sprintf(ingressesNamespaceFmt, namespace)
	c.servicesURI = fmt.Sprintf(servicesNamespaceFmt, namespace)
	c.endpointsURI = fmt.Sprintf(endpointsNamespaceFmt, namespace)
}

func (c *clusterClient) createRequest(uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest("GET", c.apiURL+uri, body)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
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

// filterIngressesByClass will filter only the ingresses that have the valid class, these are
// the defined one, empty string class or not class at all
func (c *clusterClient) filterIngressesByClass(items []*ingressItem) []*ingressItem {
	validIngs := []*ingressItem{}

	for _, ing := range items {
		// No metadata is the same as no annotations for us
		if ing.Metadata != nil {
			cls, ok := ing.Metadata.Annotations[ingressClassKey]
			// Skip loop iteration if not valid ingress (non defined, empty or non defined one)
			if ok && cls != "" && !c.ingressClass.MatchString(cls) {
				continue
			}
		}
		validIngs = append(validIngs, ing)
	}

	return validIngs
}

func (c *clusterClient) loadIngresses() ([]*ingressItem, error) {
	var il ingressList
	if err := c.getJSON(c.ingressesURI, &il); err != nil {
		log.Debugf("requesting all ingresses failed: %v", err)
		return nil, err
	}

	log.Debugf("all ingresses received: %d", len(il.Items))
	fItems := c.filterIngressesByClass(il.Items)
	log.Debugf("filtered ingresses by ingress class: %d", len(fItems))

	sort.Slice(fItems, func(i, j int) bool {
		mI := fItems[i].Metadata
		mJ := fItems[j].Metadata
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

	return fItems, nil
}

func (c *clusterClient) loadServices() (map[resourceId]*service, error) {
	var services serviceList
	if err := c.getJSON(c.servicesURI, &services); err != nil {
		log.Debugf("requesting all services failed: %v", err)
		return nil, err
	}

	log.Debugf("all services received: %d", len(services.Items))
	result := make(map[resourceId]*service)
	for _, service := range services.Items {
		result[service.Meta.toResourceId()] = service
	}

	return result, nil
}

func (c *clusterClient) loadEndpoints() (map[resourceId]*endpoint, error) {
	var endpoints endpointList
	if err := c.getJSON(c.endpointsURI, &endpoints); err != nil {
		log.Debugf("requesting all endpoints failed: %v", err)
		return nil, err
	}

	log.Debugf("all endpoints received: %d", len(endpoints.Items))
	result := make(map[resourceId]*endpoint)
	for _, endpoint := range endpoints.Items {
		result[endpoint.Meta.toResourceId()] = endpoint
	}

	return result, nil
}

func (c *clusterClient) fetchClusterState() (*clusterState, error) {
	ingresses, err := c.loadIngresses()
	if err != nil {
		return nil, err
	}

	services, err := c.loadServices()
	if err != nil {
		return nil, err
	}

	endpoints, err := c.loadEndpoints()
	if err != nil {
		return nil, err
	}

	return &clusterState{
		ingresses:       ingresses,
		services:        services,
		endpoints:       endpoints,
		cachedEndpoints: make(map[endpointId][]string),
	}, nil
}
