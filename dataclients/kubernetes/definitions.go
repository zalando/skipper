package kubernetes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type resourceID struct {
	namespace string
	name      string
}

type metadata struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Created     time.Time         `json:"creationTimestamp"`
	Uid         string            `json:"uid"`
	Annotations map[string]string `json:"annotations"`
}

func namespaceString(ns string) string {
	if ns == "" {
		return "default"
	}

	return ns
}

func (meta *metadata) toResourceID() resourceID {
	return resourceID{
		namespace: namespaceString(meta.Namespace),
		name:      meta.Name,
	}
}

type backendPort struct {
	value interface{}
}

var errInvalidPortType = errors.New("invalid port type")

func (p backendPort) name() (string, bool) {
	s, ok := p.value.(string)
	return s, ok
}

func (p backendPort) number() (int, bool) {
	i, ok := p.value.(int)
	return i, ok
}

func (p *backendPort) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			return err
		}

		p.value = s
		return nil
	}

	var i int
	if err := json.Unmarshal(value, &i); err != nil {
		return err
	}

	p.value = i
	return nil
}

func (p backendPort) MarshalJSON() ([]byte, error) {
	switch p.value.(type) {
	case string, int:
		return json.Marshal(p.value)
	default:
		return nil, errInvalidPortType
	}
}

func (p backendPort) String() string {
	switch v := p.value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

type backend struct {
	ServiceName string      `json:"serviceName"`
	ServicePort backendPort `json:"servicePort"`
	// Traffic field used for custom traffic weights, but not part of the ingress spec.
	Traffic float64
	// number of True predicates to add to support multi color traffic switching
	noopCount int
}

func (b backend) String() string {
	return fmt.Sprintf("svc(%s, %s) %0.2f", b.ServiceName, b.ServicePort, b.Traffic)
}

type pathRule struct {
	Path    string   `json:"path"`
	Backend *backend `json:"backend"`
}

type httpRule struct {
	Paths []*pathRule `json:"paths"`
}

type rule struct {
	Host string    `json:"host"`
	Http *httpRule `json:"http"`
}

type ingressSpec struct {
	DefaultBackend *backend `json:"backend"`
	Rules          []*rule  `json:"rules"`
}

type ingressItem struct {
	Metadata *metadata    `json:"metadata"`
	Spec     *ingressSpec `json:"spec"`
}

type ingressList struct {
	Items []*ingressItem `json:"items"`
}

type servicePort struct {
	Name       string       `json:"name"`
	Port       int          `json:"port"`
	TargetPort *backendPort `json:"targetPort"` // string or int
}

func (sp servicePort) MatchingPort(svcPort backendPort) bool {
	s := svcPort.String()
	spt := strconv.Itoa(sp.Port)
	return s != "" && (spt == s || sp.Name == s)
}

func (sp servicePort) String() string {
	return fmt.Sprintf("%s %d %s", sp.Name, sp.Port, sp.TargetPort)
}

type serviceSpec struct {
	Type         string         `json:"type"`
	ClusterIP    string         `json:"clusterIP"`
	ExternalName string         `json:"externalName"`
	Ports        []*servicePort `json:"ports"`
}

type service struct {
	Meta *metadata    `json:"metadata"`
	Spec *serviceSpec `json:"spec"`
}

type serviceList struct {
	Items []*service `json:"items"`
}

func (s service) getTargetPort(svcPort backendPort) (string, error) {
	for _, sp := range s.Spec.Ports {
		if sp.MatchingPort(svcPort) && sp.TargetPort != nil {
			return sp.TargetPort.String(), nil
		}
	}
	return "", fmt.Errorf("getTargetPort: target port not found %v given %s", s.Spec.Ports, svcPort)
}

type endpoint struct {
	Meta    *metadata `json:"metadata"`
	Subsets []*subset `json:"subsets"`
}

type endpointList struct {
	Items []*endpoint `json:"items"`
}

func (ep endpoint) targets(svcPortName, svcPortTarget string) []string {
	result := make([]string, 0)
	for _, s := range ep.Subsets {
		for _, port := range s.Ports {
			if port.Name == svcPortName || strconv.Itoa(port.Port) == svcPortTarget {
				for _, addr := range s.Addresses {
					result = append(result, fmt.Sprintf("http://%s:%d", addr.IP, port.Port))
				}
			}
		}
	}
	return result
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

func newResourceID(namespace, name string) resourceID {
	return resourceID{namespace: namespace, name: name}
}

type endpointID struct {
	resourceID
	servicePort string
	targetPort  string
}
