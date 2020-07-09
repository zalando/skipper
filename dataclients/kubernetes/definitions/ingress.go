package definitions

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var errInvalidPortType = errors.New("invalid port type")

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
