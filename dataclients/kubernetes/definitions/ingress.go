package definitions

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var errInvalidPortType = errors.New("invalid port type")

// IngressSpecV1 https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressspec-v1-networking-k8s-io
type IngressV1Spec struct {
	DefaultBackend   *BackendV1 `json:"defaultBackend,omitempty"`
	IngressClassName string     `json:"ingressClassName,omitempty"`
	Rules            []*RuleV1  `json:"rules"`
	// Ingress TLS not supported: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressspec-v1-networking-k8s-io
}

// BackendV1 https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#ingressbackend-v1-networking-k8s-io
type BackendV1 struct {
	Service Service `json:"service,omitempty"` // can be nil, because of TypedLocalObjectReference
	// Resource TypedLocalObjectReference is not supported https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#typedlocalobjectreference-v1-core

	// Traffic field used for custom traffic weights, but not part of the ingress spec.
	Traffic float64
	// number of True predicates to add to support multi color traffic switching
	NoopCount int
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

// IngressSpec is the v1beta1
type IngressSpec struct {
	DefaultBackend *Backend `json:"backend"`
	Rules          []*Rule  `json:"rules"`
}

type Backend struct {
	ServiceName string      `json:"serviceName"`
	ServicePort BackendPort `json:"servicePort"`
	// Traffic field used for custom traffic weights, but not part of the ingress spec.
	Traffic float64
	// number of True predicates to add to support multi color traffic switching
	NoopCount int
}

type Rule struct {
	Host string    `json:"host"`
	Http *HTTPRule `json:"http"`
}

type BackendPort struct {
	Value interface{}
}

type HTTPRule struct {
	Paths []*PathRule `json:"paths"`
}

type PathRule struct {
	Path    string   `json:"path"`
	Backend *Backend `json:"backend"`
}

type ResourceID struct {
	Namespace string
	Name      string
}

func (b Backend) String() string {
	return fmt.Sprintf("svc(%s, %s) %0.2f", b.ServiceName, b.ServicePort, b.Traffic)
}

func (p BackendPort) Name() (string, bool) {
	s, ok := p.Value.(string)
	return s, ok
}

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
