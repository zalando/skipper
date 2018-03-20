package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

type NodeInfoClient struct {
	kubeAPIBaseURL string
	client         *http.Client
}

func buildhttpClient() *http.Client {
	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	return &http.Client{
		Timeout:   time.Second * 2,
		Transport: netTransport,
	}
}

func NewNodeInfoClient(kubeAPIBaseURL string) *NodeInfoClient {
	return &NodeInfoClient{
		kubeAPIBaseURL: kubeAPIBaseURL,
		client:         buildhttpClient(),
	}
}

type metadata struct {
	Name string `json:"name"`
}

type status struct {
	PodIP string `json:"podIP"`
}

type item struct {
	Metadata metadata `json:"metadata"`
	Status   status   `json:"status"`
}

type itemList struct {
	Items []*item `json:"items"`
}

func (c *NodeInfoClient) nodeInfoURL(namespace, applicationName string) (string, error) {
	u, err := url.Parse(c.kubeAPIBaseURL)
	if err != nil {
		return "", err
	}
	u.Path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods"
	a := make(url.Values)
	a.Add("application", applicationName)
	ls := make(url.Values)
	ls.Add("labelSelector", a.Encode())
	u.RawQuery = ls.Encode()

	return u.String(), nil
}

func (c *NodeInfoClient) GetNodeInfo(namespace string, applicationName string) ([]*NodeInfo, error) {
	u, err := c.nodeInfoURL(namespace, applicationName)
	if err != nil {
		log.Debugf("failed to build request url for %s %s: %s", namespace, applicationName, err)
		return nil, err
	}

	rsp, err := c.client.Get(u)
	if err != nil {
		log.Debugf("request to %s %s failed: %v", namespace, applicationName, err)
		return nil, err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode > http.StatusBadRequest {
		log.Debugf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
		return nil, fmt.Errorf("request failed, status: %d, %s", rsp.StatusCode, rsp.Status)
	}

	b := bytes.NewBuffer(nil)
	if _, err := io.Copy(b, rsp.Body); err != nil {
		log.Debugf("reading response body failed: %v", err)
		return nil, err
	}
	var il itemList
	err = json.Unmarshal(b.Bytes(), &il)
	if err != nil {
		return nil, err
	}
	nodes := make([]*NodeInfo, 0)
	for _, i := range il.Items {
		addr := net.ParseIP(i.Status.PodIP)
		if addr == nil {
			log.Warn(fmt.Sprintf("failed to parse the ip %s", i.Status.PodIP))
			continue
		}
		nodes = append(nodes, &NodeInfo{Name: i.Metadata.Name, Addr: addr})
	}
	return nodes, nil
}
