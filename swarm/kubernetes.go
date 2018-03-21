package swarm

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
	"os"
	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

const (
	DefaultNamespace    = "kube-system"
	DefaultEndpointName = "skipper-ingress"

	defaultKubernetesURL    = "http://localhost:8001"
	endpointURIFmt          = "/api/v1/namespaces/%s/endpoints/%s"
	serviceAccountDir       = "/var/run/secrets/kubernetes.io/serviceaccount/"
	serviceAccountTokenKey  = "token"
	serviceAccountRootCAKey = "ca.crt"
	serviceHostEnvVar       = "KUBERNETES_SERVICE_HOST"
	servicePortEnvVar       = "KUBERNETES_SERVICE_PORT"
)

var (
	errAPIServerURLNotFound = errors.New("kubernetes API server URL could not be constructed from env vars")
	errInvalidCertificate   = errors.New("invalid CA")
	errEndpointNotFound     = errors.New("endpoint not found")
)

type Client struct {
	httpClient             *http.Client
	apiURL                 string
	provideHealthcheck     bool
	provideHTTPSRedirect   bool
	token                  string
	current                map[string]*eskip.Route
	termReceived           bool
	sigs                   chan os.Signal
	ingressClass           *regexp.Regexp
	reverseSourcePredicate bool
}

func (c *Client) Get(s string) (*http.Response, error) {
	req, err := c.createRequest("GET", s, nil)
	if err != nil {
		return nil, err
	}

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("SWARM: request to %s failed: %v", s, err)
		return nil, err
	}
	return rsp, err
}

// New creates and initializes a Kubernetes DataClient.
func NewClient(kubernetesInCluster bool, kubernetesURL string) (*Client, error) {
	httpClient, err := buildHTTPClient(serviceAccountDir+serviceAccountRootCAKey, kubernetesInCluster)
	if err != nil {
		return nil, err
	}

	apiURL, err := buildAPIURL(kubernetesInCluster, kubernetesURL)
	if err != nil {
		return nil, err
	}

	token, err := readServiceAccountToken(serviceAccountDir+serviceAccountTokenKey, kubernetesInCluster)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: httpClient,
		apiURL:     apiURL,
		token:      token,
	}, nil
}

func buildHTTPClient(certFilePath string, inCluster bool) (*http.Client, error) {
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

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    certPool,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

func buildAPIURL(kubernetesInCluster bool, kubernetesURL string) (string, error) {
	if !kubernetesInCluster {
		if kubernetesURL == "" {
			return defaultKubernetesURL, nil
		}
		return kubernetesURL, nil
	}

	host, port := os.Getenv(serviceHostEnvVar), os.Getenv(servicePortEnvVar)
	if host == "" || port == "" {
		return "", errAPIServerURLNotFound
	}

	return "https://" + net.JoinHostPort(host, port), nil
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

func (c *Client) getEndpoints(ns, name string) ([]string, error) {
	log.Debugf("SWARM: requesting endpoint: %s/%s", ns, name)
	url := fmt.Sprintf(endpointURIFmt, ns, name)
	var ep endpoint
	if err := c.getJSON(url, &ep); err != nil {
		return nil, err
	}

	if ep.Subsets == nil {
		return nil, errEndpointNotFound
	}

	targets := ep.Targets()
	if len(targets) == 0 {
		return nil, errEndpointNotFound
	}
	return targets, nil
}

func (c *Client) getJSON(uri string, a interface{}) error {
	url := c.apiURL + uri
	log.Debugf("SWARM: making request to: %s", url)

	req, err := c.createRequest("GET", url, nil)
	if err != nil {
		return err
	}

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("SWARM: request to %s failed: %v", url, err)
		return err
	}

	log.Debugf("SWARM: request to %s succeeded", url)
	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusNotFound {
		return errEndpointNotFound
	}

	if rsp.StatusCode != http.StatusOK {
		log.Debugf("SWARM: request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return fmt.Errorf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err2 := io.Copy(b, rsp.Body); err2 != nil {
		log.Debugf("SWARM: reading response body failed: %v", err2)
		return err2
	}

	err = json.Unmarshal(b.Bytes(), a)
	if err != nil {
		log.Debugf("SWARM: invalid response format: %v", err)
	}

	return err
}

func (c *Client) createRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req, nil
}

// types copied from dataclient/kubernetes/definitions.go
type endpoint struct {
	Subsets []*subset `json:"subsets"`
}

type subset struct {
	Addresses []*address `json:"addresses"`
	Ports     []*port    `json:"ports"`
}

type address struct {
	IP   string `json:"ip"`
	Node string `json:"nodeName"`
}

type port struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

func (ep endpoint) Targets() []string {
	result := make([]string, 0)
	for _, s := range ep.Subsets {
		for _, port := range s.Ports {
			for _, addr := range s.Addresses {
				result = append(result, fmt.Sprintf("http://%s:%d", addr.IP, port.Port))
			}
		}
	}
	return result
}
