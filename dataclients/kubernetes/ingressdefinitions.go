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

func (sp servicePort) matchingPort(svcPort backendPort) bool {
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
		if sp.matchingPort(svcPort) && sp.TargetPort != nil {
			return sp.TargetPort.String(), nil
		}
	}
	return "", fmt.Errorf("getTargetPort: target port not found %v given %v", s.Spec.Ports, svcPort)
}

func (s service) getTargetPortByValue(p int) (*backendPort, bool) {
	for _, pi := range s.Spec.Ports {
		if pi.Port == p {
			return pi.TargetPort, true
		}
	}

	return nil, false
}

type endpoint struct {
	Meta    *metadata `json:"metadata"`
	Subsets []*subset `json:"subsets"`
}

type endpointList struct {
	Items []*endpoint `json:"items"`
}

func formatEndpoint(a *address, p *port) string {
	return fmt.Sprintf("http://%s:%d", a.IP, p.Port)
}

// svPortName: name or value, coming from ingress
// svcPortTarget: name or value, coming from service target port
func (ep endpoint) targets(svcPortName, svcPortTarget string) []string {
	result := make([]string, 0)
	for _, s := range ep.Subsets {
		for _, port := range s.Ports {
			// TODO: verify if the comparison of port.Name == svcPortName is valid,
			// considering that the svcPortName is the name of service port specified
			// in ingress. The right way probably is not to use the svcPortName here,
			// since that's a reference from the ingress backend to the service, but
			// use the svcPortTarget, which can be a name or a number, to compare it
			// with the subset port.Name and port.Port, which is referenced by the
			// service target port, which can also be either a name or a port value.

			// case name:name -> ok (service port name matches)
			// case name:value -> ok (service port name matches)
			// case value:name -> not ok! (neither matches)
			// case value:value -> ok (target port matches)

			if port.Name == svcPortName || strconv.Itoa(port.Port) == svcPortTarget {
				for _, addr := range s.Addresses {
					result = append(result, formatEndpoint(addr, port))
				}
			}
		}
	}
	return result
}

func (ep endpoint) targetsByServiceTarget(serviceTarget *backendPort) []string {
	portName, named := serviceTarget.value.(string)
	portValue, byValue := serviceTarget.value.(int)
	for _, s := range ep.Subsets {
		for _, p := range s.Ports {
			if named && p.Name != portName || byValue && p.Port != portValue {
				continue
			}

			var result []string
			for _, a := range s.Addresses {
				result = append(result, formatEndpoint(a, p))
			}

			return result
		}
	}

	return nil
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

type clusterResource struct {
	Name string `json:"name"`
}

type clusterResourceList struct {

	// Items, aka "resources".
	Items []*clusterResource `json:"resources"`
}
