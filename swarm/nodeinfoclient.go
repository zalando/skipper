package swarm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"net/http"
	"time"
)

type NodeInfoClient struct {
	client *http.Client
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

func NewNodeInfoClient() NodeInfoClient {
	return NodeInfoClient{
		client: buildhttpClient(),
	}
}

type metadata struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations"`
}

type status struct {
	PodIp string `json:"podIP"`
}

type item struct {
	metadata `json:"metadata"`
	status   `json:"status"`
}

type itemList struct {
	Items []*item `json:"items"`
}

func (c *NodeInfoClient) GetNodeInfo(namespace string, applicationName string) ([]*NodeInfo, error) {
	rsp, err := c.client.Get("")
	if err != nil {
		log.Debugf("request to %s %s failed: %v", namespace, applicationName, err)
		return nil, err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode == http.StatusNotFound {
		return nil, errors.New("service not found")
	}

	if rsp.StatusCode != http.StatusOK {
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
		addr := net.ParseIP(i.PodIp)
		if addr == nil {
			log.Warn(fmt.Sprintf("failed to parse the ip %s", i.PodIp))
			continue
		}
		nodes = append(nodes, &NodeInfo{Name: i.Name, Addr: addr})
	}
	return nodes, nil
}
