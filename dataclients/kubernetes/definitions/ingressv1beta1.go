package definitions

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var errInvalidPortType = errors.New("invalid port type")

type IngressList struct {
	Items []*IngressItem `json:"items"`
}

type IngressItem struct {
	Metadata *Metadata    `json:"metadata"`
	Spec     *IngressSpec `json:"spec"`
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
	Traffic IngressBackendTraffic
}

func (b *Backend) GetServiceName() string {
	return b.ServiceName
}

func (b *Backend) GetServicePort() string {
	return b.ServicePort.String()
}

func (b *Backend) GetTraffic() *IngressBackendTraffic {
	return &b.Traffic
}

type Rule struct {
	Host string    `json:"host"`
	Http *HTTPRule `json:"http"`
}

func (r *Rule) GetHost() string {
	return r.Host
}

func (r *Rule) GetPathRules() []IngressPathRule {
	if r.Http == nil {
		return nil
	}
	ret := make([]IngressPathRule, len(r.Http.Paths))
	for i, pr := range r.Http.Paths {
		ret[i] = pr
	}
	return ret
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

func (r *PathRule) GetPath() string {
	return r.Path
}

func (r *PathRule) GetPathType() string {
	// https://kubernetes.io/docs/reference/using-api/deprecation-guide/#ingress-v122
	return "ImplementationSpecific"
}

func (r *PathRule) GetBackend() IngressBackend {
	return r.Backend
}

type ResourceID struct {
	Namespace string
	Name      string
}

func (b Backend) String() string {
	return fmt.Sprintf("svc(%s, %s) %0.2f", b.ServiceName, b.ServicePort, b.Traffic.Weight)
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

// ParseIngressJSON parse JSON into an IngressList
func ParseIngressJSON(d []byte) (IngressList, error) {
	var il IngressList
	err := json.Unmarshal(d, &il)
	return il, err
}

// ParseIngressYAML parse YAML into an IngressList
func ParseIngressYAML(d []byte) (IngressList, error) {
	var il IngressList
	err := yaml.Unmarshal(d, &il)
	return il, err
}

// TODO: implement once IngressItem has a validate method
// ValidateIngress is a no-op
func ValidateIngress(_ *IngressItem) error {
	return nil
}

// ValidateIngresses is a no-op
func ValidateIngresses(ingressList IngressList) error {
	var err error
	// discover all errors to avoid the user having to repeatedly validate
	for _, i := range ingressList.Items {
		nerr := ValidateIngress(i)
		if nerr != nil {
			name := i.Metadata.Name
			namespace := i.Metadata.Namespace
			nerr = fmt.Errorf("%s/%s: %w", name, namespace, nerr)
			err = errors.Wrap(err, nerr.Error())
		}
	}

	if err != nil {
		return err
	}

	return nil
}
