package kubernetes

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
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
	// referenced from routes or DefaultBackends.
	Backends []*skipperBackend `json:"backends"`

	// DefaultBackends should be in most cases only one default
	// backend which is applied to all routes, if no override was
	// added to a route. A special case is Traffic Switching which
	// will have more than one default backend definition.
	DefaultBackends []*backendReference `json:"defaultBackends,omitempty"`

	// Routes specifies the list of route based on path, method
	// and predicates. It defaults to catchall, if there are no
	// routes.
	Routes []*routeSpec `json:"routes,omitempty"`
}

// skipperBackendParser is an intermediate type required for parsing
// skipperBackend and adding type safety for Algorithm and Type with
// skipperBackend type.
type skipperBackendParser struct {
	// Name is the backendName that can be referenced as backendReference
	Name string `json:"name"`

	// Type is one of "service|shunt|loopback|dynamic|lb|network"
	Type string `json:"type"`

	// Address is required for Type network
	Address string `json:"address"`

	// Algorithm is required for Type lb
	Algorithm string `json:"algorithm"`

	// Endpoints is required for Type lb
	Endpoints []string `json:"endpoints"`

	// ServiceName is required for Type service
	ServiceName string `json:"serviceName"`

	// ServicePort is required for Type service
	ServicePort int `json:"servicePort"` // TODO(sszuecs): uint16, do we want to enforce it here?
}

// skipperBackend is the type safe version of skipperBackendParser
type skipperBackend struct {
	// Name is the backendName that can be referenced as backendReference
	Name string

	// Type is the parsed backend type
	Type eskip.BackendType

	// Address is required for Type network. Address follows the
	// URL spec without path, query and fragment. For example
	// https://user:password@example.org
	Address string

	// ServiceName is required for Type service
	ServiceName string

	// ServicePort is required for Type service
	ServicePort int

	// Algorithm is required for Type lb
	Algorithm loadbalancer.Algorithm

	// Endpoints is required for Type lb
	Endpoints []string
}

type backendReference struct {
	// BackendName references the skipperBackend by name
	BackendName string `json:"backendName"`

	// Weight defines the traffic weight, if there are 2 or more
	// default backends
	Weight int `json:"weight"` // TODO(sszuecs): uint16, do we want to enforce it here?
}

type routeSpec struct {
	// Path specifies Path predicate, only one of Path or PathSubtree is allowed
	Path string `json:"path,omitempty"`

	// PathSubtree specifies PathSubtree predicate, only one of Path or PathSubtree is allowed
	PathSubtree string `json:"pathSubtree,omitempty"`

	// PathRegexp can be added additionally
	PathRegexp string `json:"pathRegexp,omitempty"`

	// Backends specifies the list of backendReference that should
	// be applied to override the defaultBackends
	Backends []*backendReference `json:"backends,omitempty"`

	// Filters specifies the list of filters applied to the routeSpec
	Filters []string `json:"filters,omitempty"`

	// Predicates specifies the list of predicates applied to the routeSpec
	Predicates []string `json:"predicates,omitempty"`

	// Methods defines valid HTTP methods for the specified routeSpec
	Methods []string `json:"methods,omitempty"`
}

// adding Kubernetes specific backend types here. To be discussed.
// The main reason is to differentiate between service and external, in a way
// where we can also use the current global option to decide whether the service
// should then be converted to LB. Or shall we expect the route group already
// contain the pod endpoints, and ignore the global option for skipper?
// --> As CRD we have to lookup endpoints ourselves, maybe via kube.go
const (
	serviceBackend = eskip.LBBackend + 1 + iota
)

func backendTypeFromString(s string) (eskip.BackendType, error) {
	switch s {
	case "", "service":
		return serviceBackend, nil
	default:
		return eskip.BackendTypeFromString(s)
	}
}

func backendTypeToString(t eskip.BackendType) string {
	switch t {
	case serviceBackend:
		return "service"
	default:
		return t.String()
	}
}

// UnmarshalJSON creates a new skipperBackend, safe to be called on nil pointer
func (sb *skipperBackend) UnmarshalJSON(value []byte) error {
	var p skipperBackendParser
	if err := json.Unmarshal(value, &p); err != nil {
		return err
	}

	if p.ServicePort < 0 || p.ServicePort > 2<<16-1 {
		return fmt.Errorf("failed to validate ServicePort, should be in range uint16")
	}

	bt, err := backendTypeFromString(p.Type)
	if err != nil {
		return err
	}

	a, err := loadbalancer.AlgorithmFromString(p.Algorithm)
	if err != nil {
		return err
	}

	var b skipperBackend
	b.Name = p.Name
	b.Type = bt
	b.Address = p.Address
	b.ServiceName = p.ServiceName
	b.ServicePort = p.ServicePort
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
	p.Algorithm = sb.Algorithm.String()
	p.Endpoints = sb.Endpoints
	return json.Marshal(p)
}
