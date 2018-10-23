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

	log "github.com/sirupsen/logrus"
)

const (
	// DefaultNamespace is the default namespace where swarm searches for peer information
	DefaultNamespace = "kube-system"
	// DefaultLabelSelectorKey is the default label key to select Pods for peer information
	DefaultLabelSelectorKey = "application"
	// DefaultLabelSelectorValue is the default label value to select Pods for peer information
	DefaultLabelSelectorValue = "skipper-ingress"

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

// KubernetesOptions are Kubernetes specific swarm options, that are
// needed to find peers.
type KubernetesOptions struct {
	KubernetesInCluster  bool
	KubernetesAPIBaseURL string
	Namespace            string
	LabelSelectorKey     string
	LabelSelectorValue   string
}

// ClientKubernetes is the client to access kubernetes resources to find the
// peers to join a swarm.
type ClientKubernetes struct {
	httpClient *http.Client
	apiURL     string
	token      string
}

// Get does the http GET call to kubernetes API to find the initial
// peers of a swarm.
func (c *ClientKubernetes) Get(s string) (*http.Response, error) {
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

// NewClientKubernetes creates and initializes a Kubernetes client to
// find peers. A partial copy of the Kubernetes dataclient.
func NewClientKubernetes(kubernetesInCluster bool, kubernetesURL string) (*ClientKubernetes, error) {
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

	return &ClientKubernetes{
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

func (c *ClientKubernetes) getJSON(uri string, a interface{}) error {
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

func (c *ClientKubernetes) createRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return req, nil
}

// The following types and code are copied from dataclient/kubernetes/definitions.go
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
