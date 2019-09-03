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
	// Hosts specifies the host headers, that will be matched for
	// all routes created by this route group. No hosts mean
	// catchall.
	Hosts []string `json:"hosts,omitempty"`
	// Backends specify the list of backends that can be
	// referenced from routes. There should be in most cases only
	// one default backend which is aplied to all routes, if no
	// override was added to a route. A special case is Traffic
	// Switching which will have more than one backend definition
	// marked as default.
	Backends []*skipperBackend `json:"backends"`
	// Routes specifies the list of route based on path, method
	// and predicates. It defaults to catchall, if there are no
	// routes.
	Routes []*routeSpec `json:"routes,omitempty"`
}

type routeSpec struct {
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

// required for parsing skipperBackend and adding type safety for
// Algorithm and Type with skipperBackend type
type skipperBackendParser struct {
	Name string `json:"name"`
	// Type is one of "service|shunt|loopback|lb|url"
	Type string `json:"type"`
	// Default backends will be applied to all routes without overrides
	Default bool `json:"default"`
	// Weight defines the traffic weight, if there are 2 or more
	// default backends
	Weight int `json:"weight"`
	// URL is only used for Type url
	URL string `json:"url"`
	// ServiceName is only used for Type service
	ServiceName string `json:"serviceName"`
	// ServicePort is only used for Type service
	ServicePort int `json:"servicePort"`
	// Algorithm is only used for Type lb
	Algorithm string `json:"algorithm"`
	// Algorithm is only used for Type lb
	Endpoints []string `json:"endpoints"`
}

// type safe version of skipperBackendParser
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
