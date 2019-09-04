package kubernetes

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/skipper/eskip"
)

type routeGroupList struct {
	Items []*routeGroupItem `json:"items"`
}

type routeGroupItem struct {
	Metadata *metadata       `json:"metadata"`
	Spec     *routeGroupSpec `json:"spec"`
}

type routeGroupSpec struct {
	Hosts    []string          `json:"hosts,omitempty"`
	Backends []*skipperBackend `json:"backends"`
	Paths    []*pathSpec       `json:"paths"`
}

type pathSpec struct {
	Path       string   `json:"path"`
	Method     string   `json:"method,omitempty"`
	Filters    []string `json:"filters,omitempty"`
	Predicates []string `json:"predicates,omitempty"`
	Backend    string   `json:"backend,omitempty"`
}

type algorithm int

const (
	none algorithm = iota
	roundrobin
	random
	consistentHash
)

type skipperBackendParser struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	ServiceName string   `json:"serviceName"`
	ServicePort int      `json:"servicePort"`
	Default     bool     `json:"default"`
	Algorithm   string   `json:"algorithm"`
	Endpoints   []string `json:"endpoints"`
}

// can be:
// - *backend defined in definitions.go
// - SpecialBackend string   // <shunt>, ..
type skipperBackend struct {
	Name        string
	Type        eskip.BackendType
	Default     bool
	ServiceName string
	ServicePort int
	Algorithm   algorithm
	Endpoints   []string
}

// adding Kubernetes specific backend types here. To be discussed.
// The main reason is to differentiate between service and external, in a way
// where we can also use the current global option to decide whether the service
// should then be converted to LB. Or shall we expect the route group already
// contain the pod endpoints, and ignore the global option for skipper?
const (
	serviceBackend = eskip.LBBackend + iota
	externalURL
)

// Shall we support dynamic backend? Does it make sense in this
// scenario?
func backendTypeFromString(s string) (eskip.BackendType, error) {
	switch s {
	case "", "service":
		return serviceBackend, nil
	case "external":
		return externalURL, nil
	case "shunt":
		return eskip.ShuntBackend, nil
	case "loopback":
		return eskip.LoopBackend, nil
	case "lb":
		return eskip.LBBackend, nil
	default:
		return -1, fmt.Errorf("unsupported backend type: %s", s)
	}
}

func backendTypeToString(t eskip.BackendType) string {
	switch t {
	case serviceBackend:
		return "service"
	case externalURL:
		return "external"
	default:
		return t.String()
	}
}

func algorithmFromString(s string) (algorithm, error) {
	switch s {
	case "":
		return none, nil
	case "roundrobin":
		return roundrobin, nil
	case "random":
		return random, nil
	case "consistent-hash":
		return consistentHash, nil
	default:
		return none, fmt.Errorf("unsupported algorithm: %s", s)
	}
}

func algorithmToString(a algorithm) string {
	switch a {
	case roundrobin:
		return "roundrobin"
	case random:
		return "random"
	case consistentHash:
		return "consistent-hash"
	default:
		return "unknown"
	}
}

func (sb *skipperBackend) UnmarshalJSON(value []byte) error {
	var p skipperBackendParser
	if err := json.Unmarshal(value, &p); err != nil {
		return err
	}

	bt, err := backendTypeFromString(p.Type)
	if err != nil {
		return err
	}

	a, err := algorithmFromString(p.Algorithm)
	if err != nil {
		return err
	}

	var b skipperBackend
	b.Name = p.Name
	b.Type = bt
	b.ServiceName = p.ServiceName
	b.ServicePort = p.ServicePort
	b.Default = p.Default
	b.Algorithm = a
	b.Endpoints = p.Endpoints
	*sb = b
	return nil
}

func (sb skipperBackend) MarshalJSON() ([]byte, error) {
	var p skipperBackendParser
	p.Name = sb.Name
	p.Type = backendTypeToString(sb.Type)
	p.ServiceName = sb.ServiceName
	p.ServicePort = sb.ServicePort
	p.Default = sb.Default
	p.Algorithm = algorithmToString(sb.Algorithm)
	p.Endpoints = sb.Endpoints
	return json.Marshal(p)
}
