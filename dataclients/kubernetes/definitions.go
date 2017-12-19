package kubernetes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type metadata struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations"`
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
	Name string `json:"name"`
	Port int    `json:"port"`
}

type serviceSpec struct {
	ClusterIP string         `json:"clusterIP"`
	Ports     []*servicePort `json:"ports"`
}

type service struct {
	Spec *serviceSpec `json:"spec"`
}

// subsets:
// - addresses:
//   - ip: 10.2.13.5
//     nodeName: ip-172-31-15-188.eu-central-1.compute.internal
//     targetRef:
//       kind: Pod
//       name: skipper-test-1218614873-j82nc
//       namespace: default
//       resourceVersion: "76953776"
//       uid: 772c8cfd-e400-11e7-a687-065d1a116fce
//   - ip: 10.2.52.5
//     nodeName: ip-172-31-0-136.eu-central-1.compute.internal
//     targetRef:
//       kind: Pod
//       name: skipper-test-1218614873-wgt8h
//       namespace: default
//       resourceVersion: "76953799"
//       uid: 788b51aa-e400-11e7-a687-065d1a116fce
//   ports:
//   - port: 9090
//     protocol: TCP
//   - name: ssh
//     port: 22
//     protocol: TCP
type endpoint struct {
	Subsets []*subset `json:"subsets"`
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

type subset struct {
	Addresses []*address `json:"addresses"`
	Ports     []*port    `json:"ports"`
}

type address struct {
	IP   string `json:"ip"`
	Node string `json:"nodeName"`
}

type port struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}
