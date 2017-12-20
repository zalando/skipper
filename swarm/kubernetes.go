package swarm

import (
	"errors"
	"net"
	"time"
)

const (
	DefaultNamespace       = "kube-system"
	DefaultApplicationName = "skipper-ingress"
)

type KubernetesClient interface {
	GetNodeInfo(namespace string, applicationName string) ([]*NodeInfo, error)
}

type KubernetesOptions struct {
	Namespace       string
	ApplicationName string
	Client          KubernetesClient
	FetchTimeout    time.Duration
}

type knodeResponse struct {
	self  *NodeInfo
	nodes []*NodeInfo
	err   error
}

type knodeRequest struct {
	ret chan *knodeResponse
}

type KubernetesEntryPoint struct {
	KubernetesOptions
	fetch chan *knodeResponse
	nodes chan *knodeRequest
}

var errSelfAddressNotFound = errors.New("self address not found")

func fillDefaults(o KubernetesOptions) KubernetesOptions {
	fill := func(val *string, deflt string) {
		if *val == "" {
			*val = deflt
		}
	}

	fill(&o.Namespace, DefaultNamespace)
	fill(&o.ApplicationName, DefaultApplicationName)

	return o
}

func KubernetesEntry(o KubernetesOptions) EntryPoint {
	o = fillDefaults(o)
	kep := &KubernetesEntryPoint{KubernetesOptions: o}
	go kep.control()
	return kep
}

func (kep *KubernetesEntryPoint) fetchNodes(to time.Duration) {
	<-time.After(to)
	nodes, err := kep.Client.GetNodeInfo(kep.Namespace, kep.ApplicationName)
	kep.fetch <- &knodeResponse{nodes: nodes, err: err}
}

func findSelf(n []*NodeInfo) (*NodeInfo, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for i := range addrs {
		ip, _, err := net.ParseCIDR(addrs[i].String())
		if err != nil {
			return nil, err
		}

		if addrs[i].Network() == "tcp" {
			for j := range n {
				if ip.Equal(n[j].Addr) {
					return n[j], nil
				}
			}
		}
	}

	return nil, errSelfAddressNotFound
}

func (kep *KubernetesEntryPoint) control() {
	go kep.fetchNodes(0)

	var (
		nodeReqs  <-chan *knodeRequest
		lastSelf  *NodeInfo
		lastNodes []*NodeInfo
		lastError error
	)

	for {
		select {
		case frsp := <-kep.fetch:
			// nodeReqs nil, therefore blocked until first fetch done
			nodeReqs = kep.nodes

			// initiate next fetch
			go kep.fetchNodes(kep.FetchTimeout)

			if frsp.err == nil {
				self, err := findSelf(frsp.nodes)
				if err != nil {
					lastError = frsp.err
				} else {
					lastSelf = self
					lastNodes = frsp.nodes
					lastError = nil
				}
			} else {
				lastError = frsp.err
			}
		case req := <-nodeReqs:
			req.ret <- &knodeResponse{
				self:  lastSelf,
				nodes: lastNodes,
				err:   lastError,
			}
		}
	}
}

func (kep *KubernetesEntryPoint) req() *knodeResponse {
	ret := make(chan *knodeResponse)
	req := &knodeRequest{ret: ret}
	kep.nodes <- req
	return <-req.ret
}

func (kep *KubernetesEntryPoint) Node() (*NodeInfo, error) {
	rsp := kep.req()
	return rsp.self, rsp.err
}

func (kep *KubernetesEntryPoint) Nodes() ([]*NodeInfo, error) {
	rsp := kep.req()
	return rsp.nodes, rsp.err
}
