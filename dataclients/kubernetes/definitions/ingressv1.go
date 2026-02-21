package definitions

import (
	"encoding/json"
	"errors"
	"strconv"
)

const (
	IngressFilterAnnotation    = "zalando.org/skipper-filter"
	IngressPredicateAnnotation = "zalando.org/skipper-predicate"
	IngressRoutesAnnotation    = "zalando.org/skipper-routes"
	IngressBackendAnnotation   = "zalando.org/skipper-backend"
)

var errInvalidPortType = errors.New("invalid port type")

type IngressV1List struct {
	Items []*IngressV1Item `json:"items"`
}

type IngressV1Item struct {
	Metadata *Metadata      `json:"metadata"`
	Spec     *IngressV1Spec `json:"spec"`
}

// IngressV1Spec https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressspec-v1-networking-k8s-io
type IngressV1Spec struct {
	DefaultBackend   *BackendV1 `json:"defaultBackend,omitempty"`
	IngressClassName string     `json:"ingressClassName,omitempty"`
	Rules            []*RuleV1  `json:"rules"`
	IngressTLS       []*TLSV1   `json:"tls,omitempty"`
}

// BackendV1 https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressbackend-v1-networking-k8s-io
type BackendV1 struct {
	Service Service `json:"service"` // can be nil, because of TypedLocalObjectReference
	// Resource TypedLocalObjectReference is not supported https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#typedlocalobjectreference-v1-core
}

// Service https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressservicebackend-v1-networking-k8s-io
type Service struct {
	Name string        `json:"name"`
	Port BackendPortV1 `json:"port"`
}

type BackendPortV1 struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

func (p BackendPortV1) String() string {
	if p.Number != 0 {
		return strconv.Itoa(p.Number)
	}
	return p.Name
}

type RuleV1 struct {
	Host string      `json:"host"`
	Http *HTTPRuleV1 `json:"http"`
}

type HTTPRuleV1 struct {
	Paths []*PathRuleV1 `json:"paths"`
}

// PathRuleV1 https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#httpingresspath-v1-networking-k8s-io
type PathRuleV1 struct {
	Path     string     `json:"path"`
	PathType string     `json:"pathType"`
	Backend  *BackendV1 `json:"backend"`
}

type TLSV1 struct {
	Hosts      []string `json:"hosts"`
	SecretName string   `json:"secretName"`
}

// ResourceID is a stripped down version of Metadata used to identify resources in a cache map
type ResourceID struct {
	Namespace string
	Name      string
}

// BackendPort is used for TargetPort similar to Kubernetes intOrString type
type BackendPort struct {
	Value any
}

// ParseIngressV1JSON parse JSON into an IngressV1List
func ParseIngressV1JSON(d []byte) (IngressV1List, error) {
	var il IngressV1List
	err := json.Unmarshal(d, &il)
	return il, err
}

func GetHostsFromIngressRulesV1(ing *IngressV1Item) []string {
	hostList := make([]string, 0)
	for _, i := range ing.Spec.Rules {
		hostList = append(hostList, i.Host)
	}
	return hostList
}

// String converts BackendPort to string
func (p BackendPort) String() string {
	switch v := p.Value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// Number converts BackendPort to int
func (p BackendPort) Number() (int, bool) {
	i, ok := p.Value.(int)
	return i, ok
}
func (p *BackendPort) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			return err
		}

		p.Value = s
		return nil
	}

	var i int
	if err := json.Unmarshal(value, &i); err != nil {
		return err
	}

	p.Value = i
	return nil
}

func (p BackendPort) MarshalJSON() ([]byte, error) {
	switch p.Value.(type) {
	case string, int:
		return json.Marshal(p.Value)
	default:
		return nil, errInvalidPortType
	}
}
